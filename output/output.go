// Package output centralizes the fleet's agent-output conventions so every tool
// inherits CLAUDECODE row caps + a truncated marker for free.
package output

import (
	"encoding/json"
	"os"

	"golang.org/x/term"
)

// AgentRowCap is the default row cap applied to LLM (CLAUDECODE) callers.
const AgentRowCap = 100

// AgentMode reports CLAUDECODE=1 (an LLM caller such as Claude Code).
func AgentMode() bool {
	v := os.Getenv("CLAUDECODE")
	return v != "" && v != "0" && v != "false"
}

// IsJSON forces JSON when --json, --jq, or piped (non-TTY) stdout.
func IsJSON(jsonFlag bool, jqFilter string) bool {
	return jsonFlag || jqFilter != "" || !term.IsTerminal(int(os.Stdout.Fd()))
}

// ResolveLimit returns the effective row cap: an explicit --limit (>0) always
// wins; otherwise CLAUDECODE callers get AgentRowCap, humans get 0 (unbounded).
func ResolveLimit(userLimit int) int {
	if userLimit > 0 {
		return userLimit
	}
	if AgentMode() {
		return AgentRowCap
	}
	return 0
}

// CapRows truncates rows to limit and reports whether truncation occurred.
func CapRows[T any](rows []T, limit int) ([]T, bool) {
	if limit <= 0 || len(rows) <= limit {
		return rows, false
	}
	return rows[:limit], true
}

// TruncatedMarker is the canonical envelope appended when rows were capped.
func TruncatedMarker(shown, total int) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"truncated": true, "shown": shown, "total": total})
	return b
}
