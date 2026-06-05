// Package confirm is the fleet-shared write-safety gate. Destructive verbs call
// Gate (or Require) so a mutation refuses unless the operator opts in; the
// refusal is a cierrors.APIError with Kind "write_locked" (exit 6), so callers
// and agents dispatch on kind rather than parsing strings. It takes primitives
// (not a *cobra.Command) so clicore carries no cobra dependency — each tool
// wires its own --yes/--confirm/--dry-run flags and passes cmd.OutOrStdout().
package confirm

import (
	"fmt"
	"io"
	"os"

	"github.com/nicolasacchi/clicore/cierrors"
	"golang.org/x/term"
)

// Require returns nil when yes is set, otherwise a write_locked APIError.
func Require(yes bool, action string) error {
	if yes {
		return nil
	}
	hint := "re-run with --yes (or --confirm) to proceed"
	if term.IsTerminal(int(os.Stdout.Fd())) {
		hint = "re-run with --yes to proceed, or --dry-run to preview"
	}
	return &cierrors.APIError{
		Kind:   "write_locked",
		Detail: fmt.Sprintf("%s requires confirmation", action),
		Hint:   hint,
	}
}

// Gate guards a destructive verb in one line at the top of a RunE:
//
//	if handled, err := confirm.Gate(cmd.OutOrStdout(), yesFlag, dryRunFlag,
//	    fmt.Sprintf("delete list %s", id)); handled {
//	    return err
//	}
//
// It returns handled=true when the caller should return immediately: either
// --dry-run previewed the action (err nil, exit 0) or the gate refused (err is
// a write_locked APIError, exit 6).
func Gate(out io.Writer, yes, dryRun bool, action string) (bool, error) {
	if dryRun {
		_, _ = fmt.Fprintf(out, "--dry-run: would %s, no changes made\n", action)
		return true, nil
	}
	if err := Require(yes, action); err != nil {
		return true, err
	}
	return false, nil
}
