package query

import (
	"context"
	"errors"
	"strings"
	"testing"

	spellcheckerAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/spellchecker"
	octosqlAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/sql/octosql"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	k8sPort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// suggestDS is a small pods-like data source: it resolves pods (not pdos),
// serves a status-bearing schema, and lists one object so a corrected query
// renders identifiable output.
type suggestDS struct{}

func (suggestDS) Resolve(_ context.Context, table string) (k8sPort.Resource, error) {
	switch table {
	case "pods", "pod", "po":
		return k8sPort.Resource{Name: "pods", Version: "v1", Namespaced: true}, nil
	}
	return k8sPort.Resource{}, errUnknownResource
}

func (suggestDS) Resources(context.Context) ([]k8sPort.Resource, error) {
	return []k8sPort.Resource{{Name: "pods"}, {Name: "deployments"}, {Name: "services"}}, nil
}

func (suggestDS) InferSchema(context.Context, k8sPort.Resource) ([]schema.Field, error) {
	return []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "namespace", Type: schema.FieldTypeString},
		{Name: "status", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "phase", Type: schema.FieldTypeString},
		}},
	}, nil
}

func (suggestDS) List(_ context.Context, _ k8sPort.Resource, _ k8sPort.ListOptions, pageFn func([]map[string]any) error) error {
	return pageFn([]map[string]any{
		{
			"metadata": map[string]any{"name": "pod-1", "namespace": "default"},
			"status":   map[string]any{"phase": "Running"},
		},
	})
}

func (suggestDS) Delete(context.Context, k8sPort.Resource, string, string, k8sPort.DeleteOptions) error {
	return nil
}

var errUnknownResource = &resourceErr{}

type resourceErr struct{}

func (*resourceErr) Error() string { return "unknown resource" }

func newSuggestCmd(in string, stdinIsTTY, inREPL bool, w *strings.Builder) *QueryCommand {
	ds := suggestDS{}
	return &QueryCommand{
		config:     api.Config{Output: "csv", Out: w},
		k8s:        ds,
		engines:    octosqlAdapter.NewFactory(ds, spellcheckerAdapter.New()),
		in:         strings.NewReader(in),
		stdinIsTTY: stdinIsTTY,
		inREPL:     inREPL,
	}
}

// --- sql-execution (one-shot) scenarios -------------------------------------

func TestOneShot_InteractiveSuggestsAndRunsOnConfirm(t *testing.T) {
	var buf strings.Builder
	c := newSuggestCmd("y\n", true, false, &buf)
	if err := c.RunWithWriter(context.Background(), "SELECT staus FROM pods", &buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "run this query instead ? SELECT status FROM pods") {
		t.Errorf("missing suggestion line, got:\n%s", out)
	}
	if !strings.Contains(out, "Running") {
		t.Errorf("expected corrected query result (Running), got:\n%s", out)
	}
}

func TestOneShot_TableTypoSuggestsAndRuns(t *testing.T) {
	var buf strings.Builder
	c := newSuggestCmd("y\n", true, false, &buf)
	if err := c.RunWithWriter(context.Background(), "SELECT name FROM pdos", &buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "run this query instead ? SELECT name FROM pods") {
		t.Errorf("missing table suggestion line, got:\n%s", out)
	}
	if !strings.Contains(out, "pod-1") {
		t.Errorf("expected corrected query result (pod-1), got:\n%s", out)
	}
}

func TestOneShot_NonInteractivePrintsButDoesNotRun(t *testing.T) {
	var buf strings.Builder
	c := newSuggestCmd("", false, false, &buf)
	err := c.RunWithWriter(context.Background(), "SELECT staus FROM pods", &buf)
	if err == nil {
		t.Fatal("expected the original error (exit 1) on non-interactive session")
	}
	out := buf.String()
	if !strings.Contains(out, "run this query instead ? SELECT status FROM pods") {
		t.Errorf("missing suggestion line, got:\n%s", out)
	}
	if strings.Contains(out, "Running") {
		t.Errorf("corrected query should not have run non-interactively, got:\n%s", out)
	}
}

func TestOneShot_NoMatchKeepsOriginalError(t *testing.T) {
	var buf strings.Builder
	c := newSuggestCmd("", true, false, &buf)
	err := c.RunWithWriter(context.Background(), "SELECT xyzzy FROM pods", &buf)
	if err == nil {
		t.Fatal("expected the original error for a no-match typo")
	}
	if strings.Contains(buf.String(), "run this query instead") {
		t.Errorf("no suggestion should be printed for a no-match typo, got:\n%s", buf.String())
	}
}

func TestOneShot_InteractiveRejectionDoesNotRun(t *testing.T) {
	var buf strings.Builder
	c := newSuggestCmd("n\n", true, false, &buf)
	err := c.RunWithWriter(context.Background(), "SELECT staus FROM pods", &buf)
	if err == nil {
		t.Fatal("expected exit 1 when the user rejects the suggestion")
	}
	if strings.Contains(buf.String(), "Running") {
		t.Errorf("rejected suggestion should not run, got:\n%s", buf.String())
	}
}

// --- sql-repl scenarios ------------------------------------------------------
//
// In the REPL the command does not prompt or run the corrected query: it surfaces
// the SuggestionError so the REPL loop can pre-fill the corrected query into the
// input line for editing (see the readline adapter). Here we assert the command
// returns the structured suggestion and never runs the corrected query.

func TestREPL_FieldTypoReturnsSuggestionWithoutRunning(t *testing.T) {
	var buf strings.Builder
	c := newSuggestCmd("", true, true, &buf)
	err := c.RunWithWriter(context.Background(), "SELECT staus FROM pods", &buf)
	var se *sqlPort.SuggestionError
	if !errors.As(err, &se) {
		t.Fatalf("expected a SuggestionError in REPL mode, got %v", err)
	}
	if se.Suggestion.Kind != sqlPort.SuggestionKindField || se.Suggestion.CorrectedSQL != "SELECT status FROM pods" {
		t.Errorf("got suggestion %+v", se.Suggestion)
	}
	if strings.Contains(buf.String(), "Running") {
		t.Errorf("REPL must not run the corrected query, got:\n%s", buf.String())
	}
}

func TestREPL_KeywordTypoReturnsSuggestion(t *testing.T) {
	var buf strings.Builder
	c := newSuggestCmd("", true, true, &buf)
	err := c.RunWithWriter(context.Background(), "SLECT name FROM pods", &buf)
	var se *sqlPort.SuggestionError
	if !errors.As(err, &se) {
		t.Fatalf("expected a SuggestionError in REPL mode, got %v", err)
	}
	if se.Suggestion.Kind != sqlPort.SuggestionKindKeyword || se.Suggestion.CorrectedSQL != "SELECT name FROM pods" {
		t.Errorf("got suggestion %+v", se.Suggestion)
	}
	if strings.Contains(buf.String(), "pod-1") {
		t.Errorf("REPL must not run the corrected query, got:\n%s", buf.String())
	}
}

func TestREPL_NoMatchReturnsRawError(t *testing.T) {
	var buf strings.Builder
	c := newSuggestCmd("", true, true, &buf)
	err := c.RunWithWriter(context.Background(), "SELECT xyzzy FROM pods", &buf)
	if err == nil {
		t.Fatal("expected the original error for a no-match typo")
	}
	var se *sqlPort.SuggestionError
	if errors.As(err, &se) {
		t.Errorf("no-match typo should not produce a SuggestionError, got %+v", se.Suggestion)
	}
}
