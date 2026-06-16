package readline

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// fakeFiller captures what would be pre-filled into the next readline prompt.
type fakeFiller struct{ filled string }

func (f *fakeFiller) WriteStdin(b []byte) (int, error) {
	f.filled += string(b)
	return len(b), nil
}

func fieldSuggestionErr() *sqlPort.SuggestionError {
	return &sqlPort.SuggestionError{
		Suggestion: sqlPort.Suggestion{
			Kind:         sqlPort.SuggestionKindField,
			Typo:         "staus",
			Suggestion:   "status",
			CorrectedSQL: "SELECT status FROM pods",
		},
		Err: errors.New("octosql: typecheck: unknown variable: 'staus'"),
	}
}

// TestHandleInteractiveSuggestion_PrefillsCorrectedQuery verifies the REPL prints
// the diagnostic and pre-fills the corrected query for editing instead of
// repeating it in the message or prompting.
func TestHandleInteractiveSuggestion_PrefillsCorrectedQuery(t *testing.T) {
	se := fieldSuggestionErr()
	var f fakeFiller
	var errOut strings.Builder

	handleInteractiveSuggestion(se, &f, &errOut)

	if f.filled != "SELECT status FROM pods" {
		t.Errorf("pre-filled input = %q, want the corrected query", f.filled)
	}
	if !strings.Contains(errOut.String(), "did you mean status?") {
		t.Errorf("missing diagnostic hint, got %q", errOut.String())
	}
	if strings.Contains(errOut.String(), "SELECT status FROM pods") {
		t.Errorf("diagnostic should not repeat the corrected query (it is pre-filled), got %q", errOut.String())
	}
}

// TestRunBatch_SuggestionContinuesWithoutRunning verifies batch mode prints the
// suggestion and moves on without auto-running the corrected query (no editable
// prompt is available off a TTY).
func TestRunBatch_SuggestionContinuesWithoutRunning(t *testing.T) {
	var executed []string
	cfg := NewReadlineShell{
		IsTTY: false,
		IOIn:  strings.NewReader("SELECT staus FROM pods\nSELECT name FROM pods\n"),
		IOOut: io.Discard,
		RunQuery: func(_ context.Context, query string, _ io.Writer) error {
			executed = append(executed, query)
			if query == "SELECT staus FROM pods" {
				return fieldSuggestionErr()
			}
			return nil
		},
	}
	if err := cfg.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(executed) != 2 || executed[0] != "SELECT staus FROM pods" || executed[1] != "SELECT name FROM pods" {
		t.Errorf("expected to continue without running the correction, executed: %v", executed)
	}
}

// TestRunBatch_ExecutesEachQuery verifies that batch mode executes every
// non-empty line and writes output to w.
func TestRunBatch_ExecutesEachQuery(t *testing.T) {
	var executed []string
	var out strings.Builder

	cfg := NewReadlineShell{
		IsTTY: false,
		IOIn:  strings.NewReader("SELECT name FROM pods\n\nSELECT name FROM nodes\n"),
		IOOut: &out,
		RunQuery: func(_ context.Context, query string, w io.Writer) error {
			executed = append(executed, query)
			_, _ = io.WriteString(w, "ran:"+query+"\n")
			return nil
		},
	}

	if err := cfg.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(executed) != 2 {
		t.Fatalf("expected 2 queries executed, got %d: %v", len(executed), executed)
	}
	if executed[0] != "SELECT name FROM pods" || executed[1] != "SELECT name FROM nodes" {
		t.Errorf("unexpected queries: %v", executed)
	}
	if !strings.Contains(out.String(), "ran:SELECT name FROM pods") ||
		!strings.Contains(out.String(), "ran:SELECT name FROM nodes") {
		t.Errorf("output missing query results: %q", out.String())
	}
}

// TestRunBatch_ContinuesAfterError verifies a failing query does not abort the
// batch — subsequent queries still run.
func TestRunBatch_ContinuesAfterError(t *testing.T) {
	var executed []string
	cfg := NewReadlineShell{
		IsTTY: false,
		IOIn:  strings.NewReader("BAD\nGOOD\n"),
		IOOut: io.Discard,
		RunQuery: func(_ context.Context, query string, _ io.Writer) error {
			executed = append(executed, query)
			if query == "BAD" {
				return io.ErrUnexpectedEOF
			}
			return nil
		},
	}
	if err := cfg.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(executed) != 2 {
		t.Fatalf("expected both queries to run despite error, got %v", executed)
	}
}

// TestHandleCommand_Dispatch covers exit aliases, slash dispatch, and the fact
// that legacy backslash commands are no longer recognized (they pass through to
// the SQL engine instead of being handled).
func TestHandleCommand_Dispatch(t *testing.T) {
	cases := []struct {
		input       string
		wantHandled bool
		wantExit    bool
	}{
		{"/quit", true, true},
		{"quit", true, true},
		{"exit", true, true},
		{"EXIT", true, true},
		{"/help", true, false},
		{"/HELP", true, false},
		{"/clear", true, false},
		{"/history-clear", true, false},
		{"/foo", true, false}, // unknown slash command: handled, no exit, not SQL
		// Legacy backslash commands are no longer recognized -> not handled
		// (left for the SQL engine, which reports the error).
		{`\q`, false, false},
		{`\help`, false, false},
		{"?", false, false},
		{"SELECT name FROM pods", false, false},
	}
	for _, tc := range cases {
		s := &NewReadlineShell{
			IOOut:    io.Discard,
			RunQuery: func(context.Context, string, io.Writer) error { return nil },
		}
		handled, exit := s.handleCommand(context.Background(), tc.input, io.Discard)
		if handled != tc.wantHandled || exit != tc.wantExit {
			t.Errorf("handleCommand(%q) = (handled=%v, exit=%v), want (handled=%v, exit=%v)",
				tc.input, handled, exit, tc.wantHandled, tc.wantExit)
		}
	}
}

// TestHandleCommand_Help verifies /help lists the slash commands.
func TestHandleCommand_Help(t *testing.T) {
	s := &NewReadlineShell{IOOut: io.Discard}
	var out strings.Builder
	handled, exit := s.handleCommand(context.Background(), "/help", &out)
	if !handled || exit {
		t.Fatalf("/help = (handled=%v, exit=%v), want (true, false)", handled, exit)
	}
	for _, cmd := range []string{"/quit", "/clear", "/history-clear", "/help", "/version", "/tables"} {
		if !strings.Contains(out.String(), cmd) {
			t.Errorf("/help output missing %q: %q", cmd, out.String())
		}
	}
}

// TestHandleCommand_HistoryClear verifies /history-clear invokes the wired
// history reset on a TTY and is a no-op (no panic) off a TTY.
func TestHandleCommand_HistoryClear(t *testing.T) {
	t.Run("invokes reset when wired", func(t *testing.T) {
		var reset int
		s := &NewReadlineShell{clearHistory: func() { reset++ }}
		handled, exit := s.handleCommand(context.Background(), "/history-clear", io.Discard)
		if !handled || exit {
			t.Fatalf("/history-clear = (handled=%v, exit=%v), want (true, false)", handled, exit)
		}
		if reset != 1 {
			t.Errorf("/history-clear called reset %d times, want 1", reset)
		}
	})
	t.Run("no-op when not wired", func(t *testing.T) {
		s := &NewReadlineShell{} // clearHistory nil (off a TTY)
		handled, exit := s.handleCommand(context.Background(), "/history-clear", io.Discard)
		if !handled || exit {
			t.Fatalf("/history-clear = (handled=%v, exit=%v), want (true, false)", handled, exit)
		}
	})
}

// TestHandleCommand_Version verifies /version prints the version and project URL,
// defaulting to "dev" when no version is injected.
func TestHandleCommand_Version(t *testing.T) {
	t.Run("default dev", func(t *testing.T) {
		s := &NewReadlineShell{}
		var out strings.Builder
		s.handleCommand(context.Background(), "/version", &out)
		if !strings.Contains(out.String(), "dev") {
			t.Errorf("default /version should print 'dev': %q", out.String())
		}
		if !strings.Contains(out.String(), "https://github.com/ebuildy/kubectl-sql") {
			t.Errorf("/version should print the project URL: %q", out.String())
		}
	})
	t.Run("injected version", func(t *testing.T) {
		s := &NewReadlineShell{Version: "v1.2.3", ProjectURL: "https://example.test/kubectl-sql"}
		var out strings.Builder
		s.handleCommand(context.Background(), "/version", &out)
		if !strings.Contains(out.String(), "v1.2.3") {
			t.Errorf("/version should print injected version: %q", out.String())
		}
		if !strings.Contains(out.String(), "https://example.test/kubectl-sql") {
			t.Errorf("/version should print configured URL: %q", out.String())
		}
	})
}

// TestHandleCommand_Tables verifies /tables dispatches "SHOW TABLES" through
// RunQuery so its output is identical to the SQL statement.
func TestHandleCommand_Tables(t *testing.T) {
	var gotQuery string
	s := &NewReadlineShell{
		RunQuery: func(_ context.Context, query string, w io.Writer) error {
			gotQuery = query
			_, _ = io.WriteString(w, "table-listing")
			return nil
		},
	}
	var out strings.Builder
	handled, exit := s.handleCommand(context.Background(), "/tables", &out)
	if !handled || exit {
		t.Fatalf("/tables = (handled=%v, exit=%v), want (true, false)", handled, exit)
	}
	if gotQuery != "SHOW TABLES" {
		t.Errorf("/tables dispatched %q, want %q", gotQuery, "SHOW TABLES")
	}
	if out.String() != "table-listing" {
		t.Errorf("/tables output = %q, want the SHOW TABLES output", out.String())
	}
}

// TestHandleCommand_Clear verifies /clear emits the clear sequence on a TTY and
// nothing off a TTY.
func TestHandleCommand_Clear(t *testing.T) {
	t.Run("tty clears screen", func(t *testing.T) {
		s := &NewReadlineShell{IsTTY: true}
		var out strings.Builder
		s.handleCommand(context.Background(), "/clear", &out)
		if out.String() != clearScreen {
			t.Errorf("/clear on TTY = %q, want clear sequence", out.String())
		}
	})
	t.Run("no tty is a no-op", func(t *testing.T) {
		s := &NewReadlineShell{IsTTY: false}
		var out strings.Builder
		handled, exit := s.handleCommand(context.Background(), "/clear", &out)
		if !handled || exit {
			t.Fatalf("/clear = (handled=%v, exit=%v), want (true, false)", handled, exit)
		}
		if out.String() != "" {
			t.Errorf("/clear off a TTY should emit nothing, got %q", out.String())
		}
	})
}

// TestNormalizeQuery covers trimming and trailing-semicolon stripping.
func TestNormalizeQuery(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"SELECT name FROM pods;", "SELECT name FROM pods"},
		{"  SELECT name FROM pods ;  ", "SELECT name FROM pods"},
		{"SELECT name FROM pods", "SELECT name FROM pods"},
		{"   ", ""},
		{";", ""},
	}
	for _, tc := range cases {
		if got := normalizeQuery(tc.in); got != tc.want {
			t.Errorf("normalizeQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestSemicolonStrippedBeforeExecution verifies the executor receives a query
// with no trailing semicolon (batch path exercises normalizeQuery).
func TestSemicolonStrippedBeforeExecution(t *testing.T) {
	var got string
	cfg := NewReadlineShell{
		IsTTY: false,
		IOIn:  strings.NewReader("SELECT name FROM pods;\n"),
		IOOut: io.Discard,
		RunQuery: func(_ context.Context, query string, _ io.Writer) error {
			got = query
			return nil
		},
	}
	if err := cfg.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got != "SELECT name FROM pods" {
		t.Errorf("query passed to executor = %q, want %q", got, "SELECT name FROM pods")
	}
}
