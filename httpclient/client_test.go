package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestShouldRetryNetwork(t *testing.T) {
	netErr := errors.New("connection reset")
	cases := []struct {
		name   string
		method string
		err    error
		want   bool
	}{
		{"nil err never retries", http.MethodGet, nil, false},
		{"GET network err retries", http.MethodGet, netErr, true},
		{"PUT/DELETE retry", http.MethodDelete, netErr, true},
		{"POST never retries", http.MethodPost, netErr, false},
		{"PATCH never retries", http.MethodPatch, netErr, false},
		{"permanent err never retries (even GET)", http.MethodGet, context.Canceled, false},
	}
	for _, tc := range cases {
		if got := ShouldRetryNetwork(tc.method, tc.err); got != tc.want {
			t.Errorf("%s: ShouldRetryNetwork(%s) = %v, want %v", tc.name, tc.method, got, tc.want)
		}
	}
}

type noAuth struct{}

func (noAuth) Apply(context.Context, *http.Request, int) error { return nil }
func (noAuth) OnUnauthorized() bool                            { return false }

func TestRetry_429ThenSuccess_AnyMethod(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, noAuth{}, WithHTTPClient(srv.Client()))
	_, resp, err := c.Do(context.Background(), http.MethodPost, srv.URL, []byte(`{}`), "application/json")
	if err != nil {
		t.Fatalf("want success after 429 retry, got %v", err)
	}
	if resp.StatusCode != 200 || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("want 200 in 2 calls, got %d in %d", resp.StatusCode, atomic.LoadInt32(&calls))
	}
}

// BUG-FIX #2: a POST that fails at the transport layer must NOT be retried —
// the write may already have committed server-side (duplicate-write guard).
func TestRetry_NoRetryOnPostNetworkError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		hj, _ := w.(http.Hijacker) // abruptly close to fake a mid-flight failure
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	c := New(srv.URL, noAuth{}, WithHTTPClient(srv.Client()))
	_, _, err := c.Do(context.Background(), http.MethodPost, srv.URL, []byte(`{}`), "application/json")
	if err == nil {
		t.Fatal("want network error returned, got nil")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("POST must NOT be retried on network error: want 1 call, got %d", n)
	}
}

// BUG-FIX #2 (positive): a GET IS retried on a network error.
func TestRetry_GetRetriesOnNetworkError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 2 {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, noAuth{}, WithHTTPClient(srv.Client()))
	if _, _, err := c.Do(context.Background(), http.MethodGet, srv.URL, nil, ""); err != nil {
		t.Fatalf("GET should recover after retry: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("want 2 calls, got %d", n)
	}
}

// BUG-FIX #3: a cancelled context is permanent and must not be retried.
func TestRetry_NoRetryOnPermanentError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New(srv.URL, noAuth{}, WithHTTPClient(srv.Client()))
	if _, _, err := c.Do(ctx, http.MethodGet, srv.URL, nil, ""); err == nil {
		t.Fatal("want context.Canceled returned without retry")
	}
}

// OnUnauthorized fires once on a 401 to allow a refresh, then the retry succeeds.
type refreshAuth struct{ refreshed *int32 }

func (refreshAuth) Apply(context.Context, *http.Request, int) error { return nil }
func (a refreshAuth) OnUnauthorized() bool                          { atomic.AddInt32(a.refreshed, 1); return true }

func TestRetry_RefreshOn401(t *testing.T) {
	var calls, refreshed int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, refreshAuth{&refreshed}, WithHTTPClient(srv.Client()))
	if _, _, err := c.Do(context.Background(), http.MethodGet, srv.URL, nil, ""); err != nil {
		t.Fatalf("want success after 401 refresh, got %v", err)
	}
	if r, n := atomic.LoadInt32(&refreshed), atomic.LoadInt32(&calls); r != 1 || n != 2 {
		t.Fatalf("want 1 refresh + 2 calls, got refresh=%d calls=%d", r, n)
	}
}
