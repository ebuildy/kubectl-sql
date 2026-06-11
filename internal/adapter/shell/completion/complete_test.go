package completion

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	shellCompletionPort "github.com/ebuildy/kubectl-sql/internal/port/autocomplete"
	dataSourcePort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

type fakeDataSource struct {
	tables  []string
	columns map[string][]string

	calls int32 // counts calls to Columns for caching assertions
}

func (f *fakeDataSource) Resolve(ctx context.Context, table string) (dataSourcePort.Resource, error) {
	return dataSourcePort.Resource{
		Name: table,
	}, nil
}

func (f *fakeDataSource) Resources(ctx context.Context) ([]dataSourcePort.Resource, error) {
	res := make([]dataSourcePort.Resource, 0, len(f.tables))
	for _, t := range f.tables {
		res = append(res, dataSourcePort.Resource{Name: t})
	}
	return res, nil
}

func (f *fakeDataSource) InferSchema(ctx context.Context, r dataSourcePort.Resource) ([]schema.Field, error) {
	atomic.AddInt32(&f.calls, 1)
	cols, ok := f.columns[r.Name]
	if !ok {
		return nil, fmt.Errorf("unknown resource %q", r.Name)
	}
	fields := make([]schema.Field, 0, len(cols))
	for _, c := range cols {
		fields = append(fields, schema.Field{Name: c})
	}
	return fields, nil
}

func (f *fakeDataSource) List(ctx context.Context, r dataSourcePort.Resource, opts dataSourcePort.ListOptions, pageFn func(page []map[string]any) error) error {
	return nil
}

// doString runs Do with the cursor at the end of line.
func doString(c shellCompletionPort.ShellCompletionRunner, line string) []string {
	return doAt(c, line, len([]rune(line)))
}

// doCursor runs Do with the cursor placed where the "|" marker appears in s.
// e.g. doCursor(c, "SELECT sta| FROM pods").
func doCursor(c shellCompletionPort.ShellCompletionRunner, s string) []string {
	pos := strings.Index(s, "|")
	line := strings.Replace(s, "|", "", 1)
	return doAt(c, line, len([]rune(line[:pos])))
}

// doAt runs Do at an explicit cursor position and reconstructs full candidate
// strings (typed word + returned suffix), sorted for set-membership assertions.
func doAt(c shellCompletionPort.ShellCompletionRunner, line string, pos int) []string {
	out := doOrdered(c, line, pos)
	sort.Strings(out)
	return out
}

// doOrdered is like doAt but preserves the order Do returned candidates in.
func doOrdered(c shellCompletionPort.ShellCompletionRunner, line string, pos int) []string {
	runes := []rune(line)
	suffixes, offset := c.Do(runes, pos)
	word := string(runes[pos-offset : pos])
	out := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		out = append(out, word+string(s))
	}
	return out
}

func TestComplete_KeywordCasePreserved(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{})

	lower := doString(c, "sel")
	if !contains(lower, "select") {
		t.Errorf("lowercase prefix: got %v, want to contain %q", lower, "select")
	}
	if contains(lower, "SELECT") {
		t.Errorf("lowercase prefix should not offer uppercase: %v", lower)
	}

	upper := doString(c, "SEL")
	if !contains(upper, "SELECT") {
		t.Errorf("uppercase prefix: got %v, want to contain %q", upper, "SELECT")
	}
}

func TestComplete_CappedAndSorted(t *testing.T) {
	// A table source with more than maxSuggestions matches exercises the cap.
	tables := make([]string, 0, maxSuggestions+10)
	for i := 0; i < maxSuggestions+10; i++ {
		tables = append(tables, fmt.Sprintf("res%03d", i))
	}
	c := NewShellCompletion(context.Background(), &fakeDataSource{tables: tables})

	got := doOrdered(c, "SELECT name FROM res", len("SELECT name FROM res"))
	if len(got) > maxSuggestions {
		t.Fatalf("expected at most %d suggestions, got %d", maxSuggestions, len(got))
	}
	// Verify alphabetical (case-insensitive) order.
	for i := 1; i < len(got); i++ {
		if strings.ToLower(got[i-1]) > strings.ToLower(got[i]) {
			t.Errorf("suggestions not sorted: %v", got)
			break
		}
	}
}

func TestComplete_JoinKeywords(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{})

	for _, tc := range []struct{ typed, want string }{
		{"inner", "inner join"},
		{"INNER", "INNER JOIN"},
		{"jo", "join"},
		{"left", "left join"},
		{"on", "on"},
	} {
		got := doString(c, tc.typed)
		if !contains(got, tc.want) {
			t.Errorf("typing %q: got %v, want to contain %q", tc.typed, got, tc.want)
		}
	}
}

func TestComplete_StatementStarters(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{})

	for _, tc := range []struct{ typed, want string }{
		{"sh", "show"},
		{"des", "describe"},
		{"wi", "with"},
		{"sel", "select"},
	} {
		got := doString(c, tc.typed)
		if !contains(got, tc.want) {
			t.Errorf("typing %q: got %v, want to contain %q", tc.typed, got, tc.want)
		}
	}
}

func TestComplete_EmptyLineOffersStartersUppercase(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{})
	got := doString(c, "")
	for _, kw := range []string{"SELECT", "SHOW", "DESCRIBE", "WITH"} {
		if !contains(got, kw) {
			t.Errorf("empty line: got %v, want to contain %q", got, kw)
		}
	}
}

func TestComplete_NoWriteStatements(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{})
	if got := doString(c, "up"); contains(got, "update") || contains(got, "UPDATE") {
		t.Errorf("UPDATE must not be offered (read-only): %v", got)
	}
	if got := doString(c, "del"); contains(got, "delete") || contains(got, "DELETE") {
		t.Errorf("DELETE must not be offered (read-only): %v", got)
	}
}

func TestComplete_TableAfterFrom(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{tables: []string{"pods", "podtemplates", "nodes"}})

	got := doString(c, "SELECT name FROM po")
	if !contains(got, "pods") || !contains(got, "podtemplates") {
		t.Errorf("table completion: got %v, want pods + podtemplates", got)
	}
	if contains(got, "nodes") {
		t.Errorf("table completion should filter by prefix 'po': %v", got)
	}
}

func TestComplete_TableAfterDescribeTable(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{tables: []string{"pods", "podtemplates", "nodes"}})

	got := doString(c, "DESCRIBE TABLE po")
	if !contains(got, "pods") || !contains(got, "podtemplates") {
		t.Errorf("DESCRIBE TABLE completion: got %v, want pods + podtemplates", got)
	}
	if contains(got, "nodes") {
		t.Errorf("DESCRIBE TABLE completion should filter by prefix 'po': %v", got)
	}

	// Lowercase form should also work.
	if got := doString(c, "describe table po"); !contains(got, "pods") {
		t.Errorf("lowercase describe table: got %v, want pods", got)
	}

	// All tables when nothing typed after TABLE.
	if got := doString(c, "DESCRIBE TABLE "); !contains(got, "pods") || !contains(got, "nodes") {
		t.Errorf("DESCRIBE TABLE with empty word: got %v, want all tables", got)
	}
}

func TestComplete_ColumnFromTable(t *testing.T) {
	columns := map[string][]string{"pods": {"name", "namespace", "status", "spec"}}

	c := NewShellCompletion(context.Background(), &fakeDataSource{columns: columns})

	got := doCursor(c, "SELECT sta| FROM pods")

	assert.Contains(t, got, "status", "expected 'status' column in completion candidates")
	assert.NotContains(t, got, "name", "unexpected 'name' column in completion candidates")
}

func TestComplete_UnknownTableNoColumns(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{columns: map[string][]string{}})

	// No FROM clause -> no column candidates (keywords only).
	got := doString(c, "SELECT sta")
	if contains(got, "status") {
		t.Errorf("no FROM clause should yield no columns: %v", got)
	}

	// FROM an unresolvable table -> no columns, no panic.
	got = doCursor(c, "SELECT sta| FROM doesnotexist")
	for _, g := range got {
		if g == "status" || g == "name" {
			t.Errorf("unknown table should yield no columns: %v", got)
		}
	}
}

func TestComplete_ColumnCaching(t *testing.T) {
	src := &fakeDataSource{columns: map[string][]string{"pods": {"status"}}}
	c := NewShellCompletion(context.Background(), src)

	_ = doCursor(c, "SELECT sta| FROM pods")
	_ = doCursor(c, "SELECT sta| FROM pods")
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("Columns called %d times, want 1 (cached)", got)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
