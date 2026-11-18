package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// hostOf returns the host:port of a test server, which is the form the
// program expects in a record's "ips" list (it prepends "http://").
func hostOf(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "http://")
}

// deadAddr returns an address nothing is listening on, so connections are
// refused immediately.
func deadAddr(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.NotFoundHandler())
	addr := hostOf(srv)
	srv.Close()
	return addr
}

func newBackend(t *testing.T, status int) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateAPIClient(t *testing.T) {
	t.Run("accepts a token", func(t *testing.T) {
		api, err := createAPIClient("some-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if api == nil {
			t.Fatal("expected a client")
		}
	})

	t.Run("rejects an empty token", func(t *testing.T) {
		_, err := createAPIClient("")
		if err == nil {
			t.Fatal("expected an error for an empty token")
		}
		if !strings.Contains(err.Error(), "Error creating Cloudflare client") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestCreateHTTPClient(t *testing.T) {
	t.Run("has a timeout", func(t *testing.T) {
		if got := createHTTPClient().Timeout; got != 5*time.Second {
			t.Errorf("Timeout = %v, want 5s", got)
		}
	})

	t.Run("does not follow redirects", func(t *testing.T) {
		var targetHits atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			targetHits.Add(1)
		}))
		defer target.Close()

		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusFound)
		}))
		defer redirector.Close()

		resp, err := createHTTPClient().Get(redirector.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusFound)
		}
		if n := targetHits.Load(); n != 0 {
			t.Errorf("redirect target was hit %d times, want 0", n)
		}
	})
}

func TestIsHTTPResponsive(t *testing.T) {
	client := createHTTPClient()

	// Any HTTP response at all counts as "responsive": the check is about
	// whether the host answers, not whether the app behind it is healthy.
	for _, status := range []int{200, 302, 404, 500, 503} {
		srv := newBackend(t, status)

		ok, err := isHTTPResponsive(client, hostOf(srv))
		if err != nil || !ok {
			t.Errorf("status %d: got (%v, %v), want (true, nil)", status, ok, err)
		}
	}

	t.Run("unreachable host", func(t *testing.T) {
		ok, err := isHTTPResponsive(client, deadAddr(t))
		if ok || err == nil {
			t.Errorf("got (%v, %v), want (false, error)", ok, err)
		}
	})
}

func TestGetResponsiveIP(t *testing.T) {
	client := createHTTPClient()

	t.Run("returns the first responsive IP", func(t *testing.T) {
		first := newBackend(t, 200)
		second := newBackend(t, 200)
		rec := Record{IPs: []string{hostOf(first), hostOf(second)}}

		ip, err := getResponsiveIP(client, rec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != hostOf(first) {
			t.Errorf("ip = %q, want %q", ip, hostOf(first))
		}
	})

	t.Run("skips dead IPs in order", func(t *testing.T) {
		live := newBackend(t, 200)
		rec := Record{IPs: []string{deadAddr(t), deadAddr(t), hostOf(live)}}

		ip, err := getResponsiveIP(client, rec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != hostOf(live) {
			t.Errorf("ip = %q, want %q", ip, hostOf(live))
		}
	})

	t.Run("errors when every IP is dead", func(t *testing.T) {
		rec := Record{IPs: []string{deadAddr(t), deadAddr(t)}}

		ip, err := getResponsiveIP(client, rec)
		if err == nil {
			t.Fatal("expected an error")
		}
		if ip != "" {
			t.Errorf("ip = %q, want empty", ip)
		}
		if !strings.Contains(err.Error(), "No responsive IPs found") {
			t.Errorf("unexpected error: %v", err)
		}
		// Both failures should be reported, joined by "; ".
		if got := strings.Count(err.Error(), "; "); got != 1 {
			t.Errorf("expected 2 failure messages joined by '; ', got error: %v", err)
		}
	})

	t.Run("errors when there are no IPs", func(t *testing.T) {
		_, err := getResponsiveIP(client, Record{})
		if err == nil || !strings.Contains(err.Error(), "No responsive IPs found") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
