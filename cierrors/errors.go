// Package cierrors is the fleet's canonical structured error + exit-code table,
// so every CLI emits the same machine-readable kind for agent dispatch and the
// same $? for shell/agent branching. Replaces the per-tool APIError copies that
// drifted to five mutually-incompatible exit-code schemes.
package cierrors

import "fmt"

// APIError is the transport-independent error envelope. For HTTP failures set
// Status (Kind may be derived via KindForStatus); for client-side guards
// (e.g. the write-safety gate) set Kind and leave Status zero.
type APIError struct {
	Status int    `json:"status,omitempty"`
	Kind   string `json:"kind"` // see KindForStatus + the guard kinds below
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

func (e *APIError) Error() string {
	// Client-side guard errors (no HTTP status) render from Detail/Hint.
	if e.Status == 0 && e.Kind != "" {
		msg := e.Detail
		if msg == "" {
			msg = e.Kind
		}
		if e.Hint != "" {
			return fmt.Sprintf("%s — %s", msg, e.Hint)
		}
		return msg
	}
	if e.Detail != "" {
		return fmt.Sprintf("%d: %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("API error %d", e.Status)
}

// ExitCode is the SINGLE source of truth for the fleet exit-code table:
//
//	0 success
//	1 generic / network
//	2 auth_failed / forbidden        (401, 403)
//	3 validation                     (400, bad arguments)
//	4 not_found                      (404)
//	5 rate_limited                   (429)
//	6 write_locked                   (refused mutation — confirm required)
//	7 async_timeout                  (poll/await deadline)
//
// Symbolic Kind is consulted first so a guard error maps deterministically even
// with Status 0; HTTP status is the fallback.
func (e *APIError) ExitCode() int {
	switch e.Kind {
	case "write_locked", "deprecated_endpoint", "not_publicly_documented":
		return 6
	case "async_timeout":
		return 7
	}
	switch e.Status {
	case 401, 403:
		return 2
	case 400:
		return 3
	case 404:
		return 4
	case 429:
		return 5
	}
	return 1
}

// KindForStatus maps an HTTP status to the canonical kind string.
func KindForStatus(status int) string {
	switch {
	case status == 401:
		return "auth_failed"
	case status == 403:
		return "forbidden"
	case status == 400:
		return "validation"
	case status == 404:
		return "not_found"
	case status == 429:
		return "rate_limited"
	case status >= 500:
		return "server_error"
	default:
		return "generic"
	}
}
