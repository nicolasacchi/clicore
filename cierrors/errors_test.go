package cierrors

import "testing"

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  APIError
		want int
	}{
		{"auth401", APIError{Status: 401}, 2},
		{"forbidden403", APIError{Status: 403}, 2},
		{"validation400", APIError{Status: 400}, 3},
		{"notfound404", APIError{Status: 404}, 4},
		{"ratelimited429", APIError{Status: 429}, 5},
		{"server500", APIError{Status: 500}, 1},
		{"generic", APIError{Status: 0}, 1},
		{"write_locked beats status", APIError{Kind: "write_locked"}, 6},
		{"deprecated_endpoint", APIError{Kind: "deprecated_endpoint"}, 6},
		{"async_timeout", APIError{Kind: "async_timeout"}, 7},
		{"kind wins over status", APIError{Kind: "write_locked", Status: 404}, 6},
	}
	for _, tc := range cases {
		if got := tc.err.ExitCode(); got != tc.want {
			t.Errorf("%s: ExitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestKindForStatus(t *testing.T) {
	cases := map[int]string{
		401: "auth_failed", 403: "forbidden", 400: "validation",
		404: "not_found", 429: "rate_limited", 503: "server_error", 418: "generic",
	}
	for status, want := range cases {
		if got := KindForStatus(status); got != want {
			t.Errorf("KindForStatus(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestError_GuardRendersFromDetailHint(t *testing.T) {
	e := &APIError{Kind: "write_locked", Detail: "closing 12 issues requires confirmation", Hint: "re-run with --yes"}
	if got := e.Error(); got != "closing 12 issues requires confirmation — re-run with --yes" {
		t.Errorf("guard Error() = %q", got)
	}
	if got := (&APIError{Status: 404, Detail: "not found"}).Error(); got != "404: not found" {
		t.Errorf("http Error() = %q", got)
	}
}
