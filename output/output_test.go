package output

import (
	"encoding/json"
	"testing"
)

func TestAgentMode(t *testing.T) {
	cases := map[string]bool{"1": true, "true": true, "yes": true, "": false, "0": false, "false": false}
	for v, want := range cases {
		t.Setenv("CLAUDECODE", v)
		if got := AgentMode(); got != want {
			t.Errorf("CLAUDECODE=%q: AgentMode=%v want %v", v, got, want)
		}
	}
}

func TestResolveLimit(t *testing.T) {
	t.Setenv("CLAUDECODE", "")
	if got := ResolveLimit(0); got != 0 {
		t.Errorf("human unbounded: got %d", got)
	}
	if got := ResolveLimit(25); got != 25 {
		t.Errorf("explicit limit wins: got %d", got)
	}
	t.Setenv("CLAUDECODE", "1")
	if got := ResolveLimit(0); got != AgentRowCap {
		t.Errorf("agent default cap: got %d want %d", got, AgentRowCap)
	}
	if got := ResolveLimit(10); got != 10 {
		t.Errorf("explicit limit wins even for agent: got %d", got)
	}
}

func TestCapRows(t *testing.T) {
	rows := []int{1, 2, 3, 4, 5}
	if got, trunc := CapRows(rows, 0); trunc || len(got) != 5 {
		t.Errorf("limit 0 = unbounded: got %v trunc=%v", got, trunc)
	}
	if got, trunc := CapRows(rows, 10); trunc || len(got) != 5 {
		t.Errorf("limit > len: got %v trunc=%v", got, trunc)
	}
	got, trunc := CapRows(rows, 3)
	if !trunc || len(got) != 3 {
		t.Errorf("limit < len should truncate: got %v trunc=%v", got, trunc)
	}
}

func TestTruncatedMarker(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(TruncatedMarker(3, 9), &m); err != nil {
		t.Fatal(err)
	}
	if m["truncated"] != true || m["shown"] != float64(3) || m["total"] != float64(9) {
		t.Errorf("marker = %#v", m)
	}
}
