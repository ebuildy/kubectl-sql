package query

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ebuildy/kubectl-sql/internal/port/api"
	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
)

// isDeleteStatement reports whether query is a DELETE statement (its first
// token is "delete", case-insensitive).
func isDeleteStatement(query string) bool {
	return IsDeleteStatement(query)
}

// IsDeleteStatement reports whether query is a DELETE statement (its first
// token is "delete", case-insensitive). It is exported so other entry points
// (e.g. the web UI's mutation guard) can reuse the same classifier rather than
// duplicating it.
func IsDeleteStatement(query string) bool {
	fields := strings.Fields(strings.TrimSpace(query))
	return len(fields) > 0 && strings.EqualFold(fields[0], "delete")
}

// runDelete drives the DELETE flow: resolve the deletion set, preview it, gate
// on confirmation (unless --yes / --dry-run), then delete with a progress bar
// (outside the REPL) and print a per-object result summary. The mutator is
// injected at construction by the composition root.
func (c *QueryCommand) runDelete(ctx context.Context, query string, w io.Writer) error {
	mut := c.mut

	plan, err := mut.Plan(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return fmt.Errorf("kubectl-sql: plan delete: %w", err)
	}

	// Empty match set: no preview table, no prompt, no deletion.
	if len(plan.Targets) == 0 {
		_, _ = fmt.Fprintln(w, "nothing matched; no objects to delete")
		return nil
	}

	c.printDeletePreview(w, plan)

	// --dry-run: stop after the preview, prompt nothing, delete nothing.
	if c.config.DryRun {
		_, _ = fmt.Fprintln(w, "--dry-run set; no objects were deleted")
		return nil
	}

	ok, err := c.confirmDelete(w)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return err
	}
	if !ok {
		_, _ = fmt.Fprintln(w, "deletion cancelled; no objects were deleted")
		return nil
	}

	onProgress, finish := c.deleteProgress(len(plan.Targets))
	result, err := mut.Apply(ctx, plan, onProgress)
	finish()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return fmt.Errorf("kubectl-sql: apply delete: %w", err)
	}

	c.printDeleteResult(w, result)

	if failed := result.Failed(); failed > 0 {
		return api.ExitError{Code: 2, Err: fmt.Errorf("kubectl-sql: %d object(s) failed to delete", failed)}
	}
	return nil
}

// printDeletePreview prints the total count followed by the raw list of
// equivalent kubectl delete commands (one per matched object) — each command
// already carries the object name and its -n namespace.
func (c *QueryCommand) printDeletePreview(w io.Writer, plan sqlPort.DeletePlan) {
	_, _ = fmt.Fprintf(w, "%d object(s) match and will be deleted:\n", len(plan.Targets))
	for _, cmd := range plan.KubectlCommands {
		_, _ = fmt.Fprintf(w, "  %s\n", cmd)
	}
}

// confirmDelete returns whether the deletion may proceed. --yes skips the
// prompt; on a non-interactive session without --yes it refuses (exit 1); on a
// TTY it prompts (default no).
func (c *QueryCommand) confirmDelete(w io.Writer) (bool, error) {
	if c.config.Yes {
		return true, nil
	}
	if !c.stdinIsTTY {
		return false, api.ExitError{Code: 1, Err: fmt.Errorf("kubectl-sql: refusing to delete non-interactively without --yes; pass --yes to confirm")}
	}

	_, _ = fmt.Fprint(w, "Delete these objects? [y/N]: ")
	answer, err := readLine(c.in)
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("kubectl-sql: read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// deleteProgress returns an onProgress callback and a finish function. A live
// progress bar is shown only for one-shot CLI runs on a TTY; in the REPL or
// when stdout is not a terminal it returns a no-op callback.
func (c *QueryCommand) deleteProgress(total int) (onProgress func(), finish func()) {
	if c.inREPL || !isTerminal(os.Stdout) {
		return nil, func() {}
	}
	bar := progressbar.NewOptions(total,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetDescription("deleting"),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)
	return func() { _ = bar.Add(1) }, func() { _ = bar.Finish() }
}

// printDeleteResult prints a per-object status table (preview order) and a
// deleted/failed count summary.
func (c *QueryCommand) printDeleteResult(w io.Writer, result sqlPort.DeleteResult) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"NAMESPACE", "NAME", "STATUS"})
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)
	for _, o := range result.Outcomes {
		status := "deleted"
		if o.Err != nil {
			status = "failed: " + o.Err.Error()
		}
		table.Append([]string{o.Ref.Namespace, o.Ref.Name, status})
	}
	table.Render()

	_, _ = fmt.Fprintf(w, "deleted: %d, failed: %d\n", result.Deleted(), result.Failed())
}

// readLine reads a single line (up to and excluding '\n') from r one byte at a
// time, so it never buffers past the newline — important in the REPL where the
// same underlying stdin feeds the line editor.
func readLine(r io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return sb.String(), nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			return sb.String(), err
		}
	}
}
