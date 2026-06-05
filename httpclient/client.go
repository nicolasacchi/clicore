// Package httpclient is the fleet-shared HTTP layer. It centralizes the retry
// loop so the two recurring bug classes are fixed exactly once:
//   - never retry a non-idempotent verb (POST/PATCH) on a network error
//     (a mid-flight failure may already have committed the write); and
//   - never retry a permanent error (TLS/x509, DNS NXDOMAIN, ctx cancel).
//
// A clean 429/5xx HTTP response is retried for ANY method, since the server
// rejected the request before acting on it.
package httpclient

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultMaxRetries = 3
	DefaultTimeout    = 60 * time.Second
	maxBackoff        = 60 * time.Second
)

// Authorizer injects auth onto each (re)tried request, so a refresh after a 401
// is picked up without forking the retry loop.
type Authorizer interface {
	// Apply sets auth headers. Called fresh on every attempt (attempt is 0-based).
	Apply(ctx context.Context, req *http.Request, attempt int) error
	// OnUnauthorized is invoked once on the first 401 to allow a token refresh.
	// Return true to retry the request once more.
	OnUnauthorized() bool
}

type Client struct {
	http       *http.Client
	baseURL    string
	auth       Authorizer
	maxRetries int
	verbose    bool
}

func New(baseURL string, auth Authorizer, opts ...Option) *Client {
	c := &Client{
		http:       &http.Client{Timeout: DefaultTimeout},
		baseURL:    baseURL,
		auth:       auth,
		maxRetries: DefaultMaxRetries,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type Option func(*Client)

// WithHTTPClient injects a custom *http.Client — used by tests to point at an
// httptest.Server, and by tools that need a different timeout.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }
func WithMaxRetries(n int) Option          { return func(c *Client) { c.maxRetries = n } }
func WithVerbose(v bool) Option            { return func(c *Client) { c.verbose = v } }

// BaseURL returns the configured origin.
func (c *Client) BaseURL() string { return c.baseURL }

// Response carries the status and headers of a completed request.
type Response struct {
	StatusCode int
	Header     http.Header
}

// Do executes method against url with the given body bytes (may be nil) and
// returns the response body. The retry policy is method-aware.
func (c *Client) Do(ctx context.Context, method, url string, body []byte, contentType string) ([]byte, *Response, error) {
	retriedAuth := false
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		var r io.Reader
		if len(body) > 0 {
			r = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, r)
		if err != nil {
			return nil, nil, fmt.Errorf("build request: %w", err)
		}
		if err := c.auth.Apply(ctx, req, attempt); err != nil {
			return nil, nil, err
		}
		req.Header.Set("Accept", "application/json")
		if contentType != "" && len(body) > 0 {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			// BUG-FIX #3: never retry a permanent error.
			if isPermanent(err) {
				return nil, nil, err
			}
			// BUG-FIX #2: never retry a non-idempotent verb on a network error —
			// the write may already have committed server-side.
			if !idempotent(method) {
				return nil, nil, err
			}
			if attempt == c.maxRetries {
				return nil, nil, err
			}
			if e := sleepCtx(ctx, backoff(attempt, "")); e != nil {
				return nil, nil, e
			}
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized && !retriedAuth && c.auth.OnUnauthorized() {
			_ = resp.Body.Close()
			retriedAuth = true
			continue
		}

		// A clean 429/5xx response means the request reached the server and was
		// rejected before any state change — safe to retry for ANY method.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			ra := resp.Header.Get("Retry-After")
			_ = resp.Body.Close()
			if attempt == c.maxRetries {
				return nil, &Response{StatusCode: resp.StatusCode}, fmt.Errorf("status %d after %d retries", resp.StatusCode, c.maxRetries)
			}
			if e := sleepCtx(ctx, backoff(attempt, ra)); e != nil {
				return nil, nil, e
			}
			continue
		}

		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return b, &Response{StatusCode: resp.StatusCode, Header: resp.Header}, nil
	}
	return nil, nil, lastErr
}

// ShouldRetryNetwork reports whether a transport-level error for the given HTTP
// method may be retried. It encodes the fleet's canonical policy so a tool that
// keeps its own request loop can adopt the correct behavior without a full
// rewrite: never retry a permanent error (TLS/x509, DNS NXDOMAIN, ctx cancel),
// and never retry a non-idempotent verb (POST/PATCH) — a mid-flight failure may
// already have committed the write. A clean 429/5xx HTTP *response* is a
// separate, method-independent decision (the server rejected before acting).
func ShouldRetryNetwork(method string, err error) bool {
	if err == nil || isPermanent(err) {
		return false
	}
	return idempotent(method)
}

// idempotent reports whether method is safe to retry on a network failure.
func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	default: // POST, PATCH
		return false
	}
}

// isPermanent reports errors that will never succeed on retry.
func isPermanent(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var cert x509.UnknownAuthorityError
	var host x509.HostnameError
	var inv x509.CertificateInvalidError
	if errors.As(err, &cert) || errors.As(err, &host) || errors.As(err, &inv) {
		return true
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsNotFound {
		return true
	}
	return false
}

func backoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(retryAfter); err == nil {
			if d := time.Until(t); d > 0 {
				return d
			}
		}
	}
	base := time.Duration(math.Pow(2, float64(attempt))) * time.Second
	if base > maxBackoff {
		base = maxBackoff
	}
	return base + time.Duration(rand.Int64N(int64(500*time.Millisecond)))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
