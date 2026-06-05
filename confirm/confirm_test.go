package confirm

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/nicolasacchi/clicore/cierrors"
)

func TestRequire(t *testing.T) {
	if err := Require(true, "delete x"); err != nil {
		t.Fatalf("--yes should proceed, got %v", err)
	}
	err := Require(false, "delete x")
	var apiErr *cierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != "write_locked" {
		t.Fatalf("want write_locked APIError, got %v", err)
	}
	if apiErr.ExitCode() != 6 {
		t.Fatalf("write_locked must map to exit 6, got %d", apiErr.ExitCode())
	}
}

func TestGate_DryRunPreviews(t *testing.T) {
	var buf bytes.Buffer
	handled, err := Gate(&buf, false, true, "delete list abc")
	if !handled || err != nil {
		t.Fatalf("dry-run: want handled=true err=nil, got %v / %v", handled, err)
	}
	if !strings.Contains(buf.String(), "would delete list abc") {
		t.Fatalf("dry-run should preview, got %q", buf.String())
	}
}

func TestGate_RefusesWithoutYes(t *testing.T) {
	var buf bytes.Buffer
	handled, err := Gate(&buf, false, false, "delete list abc")
	if !handled {
		t.Fatal("refusal must be handled")
	}
	var apiErr *cierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.ExitCode() != 6 {
		t.Fatalf("want write_locked exit 6, got %v", err)
	}
}

func TestGate_ProceedsWithYes(t *testing.T) {
	var buf bytes.Buffer
	handled, err := Gate(&buf, true, false, "delete list abc")
	if handled || err != nil {
		t.Fatalf("--yes: want handled=false err=nil, got %v / %v", handled, err)
	}
}
