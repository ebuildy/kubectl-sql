package query

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// handleSuggestion presents a single-token typo correction on the one-shot CLI
// path and, on interactive confirmation, runs the corrected query in place of
// the failed one. (The REPL does not use this helper: it pre-fills the corrected
// query into the input line for editing instead of prompting — see the readline
// adapter.)
//
//   - Non-interactive: print the suggestion line; never auto-run; return the
//     original error (exit 1).
//   - Interactive: prompt (default yes). On yes, run the corrected query and
//     return its result. On no, do not run and return the original error (exit 1).
func (c *QueryCommand) handleSuggestion(ctx context.Context, se *sqlPort.SuggestionError, w io.Writer) error {
	_, _ = fmt.Fprintln(w, se.Suggestion.Message())

	if !c.stdinIsTTY {
		return se.Err
	}

	ok, err := c.confirmSuggestion(w)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return err
	}
	if !ok {
		return se.Err
	}
	return c.RunWithWriter(ctx, se.Suggestion.CorrectedSQL, w)
}

// confirmSuggestion prompts whether to run the corrected query. The default
// (empty answer / Enter) is yes, since the corrected query is read-only.
func (c *QueryCommand) confirmSuggestion(w io.Writer) (bool, error) {
	_, _ = fmt.Fprint(w, "[Y/n]: ")
	answer, err := readLine(c.in)
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("kubectl-sql: read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
