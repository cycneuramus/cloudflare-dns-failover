package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	cf "github.com/cloudflare/cloudflare-go"
)

// fakeRecord is a DNS record as returned by the fake Cloudflare API.
type fakeRecord struct {
	ID      string
	Content string
}

type listRequest struct {
	Path  string
	Query url.Values
}

type updateRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

// fakeCloudflare is a minimal stand-in for the Cloudflare DNS API. It records
// every request so tests can assert on what was sent.
type fakeCloudflare struct {
	mu sync.Mutex

	records    []fakeRecord
	failList   bool
	failUpdate bool

	lists   []listRequest
	updates []updateRequest
}

func (f *fakeCloudflare) updateCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.updates)
}

func (f *fakeCloudflare) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/dns_records"):
		f.lists = append(f.lists, listRequest{Path: r.URL.Path, Query: r.URL.Query()})
		if f.failList {
			writeCFError(w)
			return
		}

		result := make([]map[string]any, 0, len(f.records))
		for _, rec := range f.records {
			result = append(result, map[string]any{"id": rec.ID, "type": "A", "content": rec.Content})
		}
		writeCFResult(w, result, len(result))

	case (r.Method == http.MethodPut || r.Method == http.MethodPatch) && strings.Contains(r.URL.Path, "/dns_records/"):
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.updates = append(f.updates, updateRequest{Method: r.Method, Path: r.URL.Path, Body: body})
		if f.failUpdate {
			writeCFError(w)
			return
		}

		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		writeCFResult(w, map[string]any{"id": id, "type": "A", "content": body["content"]}, 1)

	default:
		http.NotFound(w, r)
	}
}

func writeCFResult(w http.ResponseWriter, result any, count int) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result":   result,
		"result_info": map[string]any{
			"page": 1, "per_page": 50, "count": count, "total_count": count, "total_pages": 1,
		},
	})
}

// writeCFError uses 403 rather than 5xx so the client doesn't retry with backoff.
func writeCFError(w http.ResponseWriter) {
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"Authentication error"}],"messages":[],"result":null}`))
}

func newTestAPI(t *testing.T, fake *fakeCloudflare) *cf.API {
	t.Helper()

	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	api, err := cf.NewWithAPIToken("test-token", cf.BaseURL(srv.URL))
	if err != nil {
		t.Fatalf("creating test API client: %v", err)
	}
	return api
}

func TestFetchInitialState(t *testing.T) {
	ctx := context.Background()
	rec := Record{Domain: "*.example.com", ZoneID: "zone-1"}

	t.Run("uses the first A record", func(t *testing.T) {
		fake := &fakeCloudflare{records: []fakeRecord{
			{ID: "rec-1", Content: "10.0.0.1"},
			{ID: "rec-2", Content: "10.0.0.2"},
		}}
		api := newTestAPI(t, fake)

		state, err := fetchInitialState(ctx, api, rec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state.ActiveIP != "10.0.0.1" || state.ID != "rec-1" {
			t.Errorf("state = %+v, want ActiveIP 10.0.0.1 and ID rec-1", *state)
		}

		if len(fake.lists) != 1 {
			t.Fatalf("got %d list requests, want 1", len(fake.lists))
		}
		req := fake.lists[0]
		if !strings.Contains(req.Path, "/zones/zone-1/") {
			t.Errorf("request path %q should target zone-1", req.Path)
		}
		if got := req.Query.Get("name"); got != rec.Domain {
			t.Errorf("name query = %q, want %q", got, rec.Domain)
		}
		if got := req.Query.Get("type"); got != "A" {
			t.Errorf("type query = %q, want A", got)
		}
	})

	t.Run("errors when no A record exists", func(t *testing.T) {
		api := newTestAPI(t, &fakeCloudflare{})

		state, err := fetchInitialState(ctx, api, rec)
		if err == nil {
			t.Fatal("expected an error")
		}
		if state != nil {
			t.Errorf("state = %+v, want nil", state)
		}
		if !strings.Contains(err.Error(), "no A record found for *.example.com") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("wraps API errors with the domain", func(t *testing.T) {
		api := newTestAPI(t, &fakeCloudflare{failList: true})

		state, err := fetchInitialState(ctx, api, rec)
		if err == nil {
			t.Fatal("expected an error")
		}
		if state != nil {
			t.Errorf("state = %+v, want nil", state)
		}
		if !strings.Contains(err.Error(), "fetching DNS record for *.example.com") {
			t.Errorf("error should mention the domain, got: %v", err)
		}
	})
}

func TestEnsureIP(t *testing.T) {
	ctx := context.Background()
	rec := Record{Domain: "example.com", ZoneID: "zone-1"}

	t.Run("no-op when IP is already active", func(t *testing.T) {
		fake := &fakeCloudflare{}
		api := newTestAPI(t, fake)
		state := &DNSState{ActiveIP: "10.0.0.1", ID: "rec-1"}

		msg, err := ensureIP(ctx, api, rec, state, "10.0.0.1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg != "already set to 10.0.0.1" {
			t.Errorf("msg = %q", msg)
		}
		if n := fake.updateCount(); n != 0 {
			t.Errorf("made %d update requests, want 0", n)
		}
	})

	t.Run("updates record and state when IP changes", func(t *testing.T) {
		fake := &fakeCloudflare{}
		api := newTestAPI(t, fake)
		state := &DNSState{ActiveIP: "10.0.0.1", ID: "rec-1"}

		msg, err := ensureIP(ctx, api, rec, state, "10.0.0.2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg != "updated to 10.0.0.2 (previous: 10.0.0.1)" {
			t.Errorf("msg = %q", msg)
		}
		if state.ActiveIP != "10.0.0.2" {
			t.Errorf("state.ActiveIP = %q, want 10.0.0.2", state.ActiveIP)
		}

		if len(fake.updates) != 1 {
			t.Fatalf("got %d update requests, want 1", len(fake.updates))
		}
		req := fake.updates[0]
		if !strings.Contains(req.Path, "/zones/zone-1/dns_records/rec-1") {
			t.Errorf("update path = %q, want zone-1 / rec-1", req.Path)
		}
		if req.Body["type"] != "A" {
			t.Errorf("type = %v, want A", req.Body["type"])
		}
		if req.Body["name"] != "example.com" {
			t.Errorf("name = %v, want example.com", req.Body["name"])
		}
		if req.Body["content"] != "10.0.0.2" {
			t.Errorf("content = %v, want 10.0.0.2", req.Body["content"])
		}
		if ttl, _ := req.Body["ttl"].(float64); ttl != 1 {
			t.Errorf("ttl = %v, want 1 (automatic)", req.Body["ttl"])
		}
	})

	t.Run("sends the proxied setting from the record", func(t *testing.T) {
		for _, proxied := range []bool{false, true} {
			fake := &fakeCloudflare{}
			api := newTestAPI(t, fake)
			r := Record{Domain: "example.com", ZoneID: "zone-1", Proxied: proxied}
			state := &DNSState{ActiveIP: "10.0.0.1", ID: "rec-1"}

			if _, err := ensureIP(ctx, api, r, state, "10.0.0.2"); err != nil {
				t.Fatalf("proxied=%v: unexpected error: %v", proxied, err)
			}
			if got := fake.updates[0].Body["proxied"]; got != proxied {
				t.Errorf("proxied sent as %v, want %v", got, proxied)
			}
		}
	})

	t.Run("leaves state untouched when the update fails", func(t *testing.T) {
		fake := &fakeCloudflare{failUpdate: true}
		api := newTestAPI(t, fake)
		state := &DNSState{ActiveIP: "10.0.0.1", ID: "rec-1"}

		msg, err := ensureIP(ctx, api, rec, state, "10.0.0.2")
		if err == nil {
			t.Fatal("expected an error")
		}
		if msg != "" {
			t.Errorf("msg = %q, want empty on error", msg)
		}
		if !strings.Contains(err.Error(), "DNS update failed") {
			t.Errorf("unexpected error: %v", err)
		}
		if state.ActiveIP != "10.0.0.1" {
			t.Errorf("state.ActiveIP = %q, should still be 10.0.0.1", state.ActiveIP)
		}
	})
}
