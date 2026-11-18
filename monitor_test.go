package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	ctx := context.Background()
	httpClient := createHTTPClient()

	t.Run("reports no change when already on the responsive IP", func(t *testing.T) {
		live := newBackend(t, 200)
		fake := &fakeCloudflare{}
		api := newTestAPI(t, fake)

		rec := Record{Domain: "example.com", ZoneID: "zone-1", IPs: []string{hostOf(live)}}
		state := &DNSState{ActiveIP: hostOf(live), ID: "rec-1"}

		msg := check(ctx, api, httpClient, rec, state)

		if want := "already set to " + hostOf(live); msg != want {
			t.Errorf("msg = %q, want %q", msg, want)
		}
		if n := fake.updateCount(); n != 0 {
			t.Errorf("made %d update requests, want 0", n)
		}
	})

	t.Run("fails over to the next responsive IP", func(t *testing.T) {
		dead := deadAddr(t)
		live := newBackend(t, 200)
		fake := &fakeCloudflare{}
		api := newTestAPI(t, fake)

		rec := Record{Domain: "example.com", ZoneID: "zone-1", IPs: []string{dead, hostOf(live)}}
		state := &DNSState{ActiveIP: dead, ID: "rec-1"}

		msg := check(ctx, api, httpClient, rec, state)

		if !strings.HasPrefix(msg, "updated to "+hostOf(live)) || !strings.Contains(msg, "previous: "+dead) {
			t.Errorf("unexpected msg: %q", msg)
		}
		if state.ActiveIP != hostOf(live) {
			t.Errorf("state.ActiveIP = %q, want %q", state.ActiveIP, hostOf(live))
		}
		if n := fake.updateCount(); n != 1 {
			t.Errorf("made %d update requests, want 1", n)
		}
	})

	t.Run("fails back to a higher-priority IP once it recovers", func(t *testing.T) {
		primary := newBackend(t, 200)
		secondary := newBackend(t, 200)
		fake := &fakeCloudflare{}
		api := newTestAPI(t, fake)

		rec := Record{Domain: "example.com", ZoneID: "zone-1", IPs: []string{hostOf(primary), hostOf(secondary)}}
		state := &DNSState{ActiveIP: hostOf(secondary), ID: "rec-1"}

		msg := check(ctx, api, httpClient, rec, state)

		if !strings.HasPrefix(msg, "updated to "+hostOf(primary)) {
			t.Errorf("unexpected msg: %q", msg)
		}
		if state.ActiveIP != hostOf(primary) {
			t.Errorf("state.ActiveIP = %q, want %q", state.ActiveIP, hostOf(primary))
		}
	})

	t.Run("makes no changes when nothing is responsive", func(t *testing.T) {
		dead := deadAddr(t)
		fake := &fakeCloudflare{}
		api := newTestAPI(t, fake)

		rec := Record{Domain: "example.com", ZoneID: "zone-1", IPs: []string{dead}}
		state := &DNSState{ActiveIP: dead, ID: "rec-1"}

		msg := check(ctx, api, httpClient, rec, state)

		if !strings.Contains(msg, "No responsive IPs found") {
			t.Errorf("unexpected msg: %q", msg)
		}
		if n := fake.updateCount(); n != 0 {
			t.Errorf("made %d update requests, want 0", n)
		}
		if state.ActiveIP != dead {
			t.Errorf("state.ActiveIP changed to %q", state.ActiveIP)
		}
	})

	t.Run("reports DNS update failures and keeps state", func(t *testing.T) {
		dead := deadAddr(t)
		live := newBackend(t, 200)
		fake := &fakeCloudflare{failUpdate: true}
		api := newTestAPI(t, fake)

		rec := Record{Domain: "example.com", ZoneID: "zone-1", IPs: []string{hostOf(live)}}
		state := &DNSState{ActiveIP: dead, ID: "rec-1"}

		msg := check(ctx, api, httpClient, rec, state)

		if !strings.Contains(msg, "DNS update failed") {
			t.Errorf("unexpected msg: %q", msg)
		}
		if state.ActiveIP != dead {
			t.Errorf("state.ActiveIP = %q, should still be %q", state.ActiveIP, dead)
		}
	})
}

func TestMonitor_LogsOnlyOnChangeAndStopsOnCancel(t *testing.T) {
	var hits atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer backend.Close()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer log.SetOutput(os.Stderr)
	defer log.SetFlags(log.LstdFlags)

	api := newTestAPI(t, &fakeCloudflare{})
	rec := Record{Domain: "example.com", ZoneID: "zone-1", IPs: []string{hostOf(backend)}}
	state := &DNSState{ActiveIP: hostOf(backend), ID: "rec-1"}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitor(ctx, api, createHTTPClient(), rec, state, 5*time.Millisecond)
	}()

	// Let several check cycles run.
	deadline := time.After(2 * time.Second)
	for hits.Load() < 3 {
		select {
		case <-deadline:
			t.Fatal("monitor did not run multiple checks")
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop after context cancellation")
	}

	// Several checks ran, but the unchanged message should be logged once.
	out := buf.String()
	if got := strings.Count(out, "\n"); got != 1 {
		t.Errorf("logged %d lines, want 1:\n%s", got, out)
	}
	if !strings.Contains(out, "example.com: already set to "+hostOf(backend)) {
		t.Errorf("unexpected log output: %q", out)
	}
}
