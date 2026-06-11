package readline

import (
	"context"
	"io"
	"strings"
	"testing"
)

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

// TestHandleSlashCommand covers quit, help, and pass-through behavior.
func TestHandleSlashCommand(t *testing.T) {
	cases := []struct {
		input       string
		wantHandled bool
		wantExit    bool
	}{
		{`\q`, true, true},
		{"quit", true, true},
		{"exit", true, true},
		{"EXIT", true, true},
		{`\help`, true, false},
		{"?", true, false},
		{"SELECT name FROM pods", false, false},
	}
	for _, tc := range cases {
		var out strings.Builder
		handled, exit := handleSlashCommand(tc.input, &out)
		if handled != tc.wantHandled || exit != tc.wantExit {
			t.Errorf("handleSlashCommand(%q) = (handled=%v, exit=%v), want (handled=%v, exit=%v)",
				tc.input, handled, exit, tc.wantHandled, tc.wantExit)
		}
		if tc.input == `\help` || tc.input == "?" {
			if !strings.Contains(out.String(), `\q`) || !strings.Contains(out.String(), `\help`) {
				t.Errorf("help output missing commands: %q", out.String())
			}
		}
	}
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
