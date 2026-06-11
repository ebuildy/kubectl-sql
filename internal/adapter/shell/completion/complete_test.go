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
	fields  map[string][]schema.Field // table -> fields with SubFields, takes precedence over columns

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
	if fields, ok := f.fields[r.Name]; ok {
		return fields, nil
	}
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
	c := NewShellCompletion(context.Background(), &fakeDataSource{}, nil)

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
	c := NewShellCompletion(context.Background(), &fakeDataSource{tables: tables}, nil)

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
	c := NewShellCompletion(context.Background(), &fakeDataSource{}, nil)

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
	c := NewShellCompletion(context.Background(), &fakeDataSource{}, nil)

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
	c := NewShellCompletion(context.Background(), &fakeDataSource{}, nil)
	got := doString(c, "")
	for _, kw := range []string{"SELECT", "SHOW", "DESCRIBE", "WITH"} {
		if !contains(got, kw) {
			t.Errorf("empty line: got %v, want to contain %q", got, kw)
		}
	}
}

func TestComplete_NoWriteStatements(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{}, nil)
	if got := doString(c, "up"); contains(got, "update") || contains(got, "UPDATE") {
		t.Errorf("UPDATE must not be offered (read-only): %v", got)
	}
	if got := doString(c, "del"); contains(got, "delete") || contains(got, "DELETE") {
		t.Errorf("DELETE must not be offered (read-only): %v", got)
	}
}

func TestComplete_FunctionNames(t *testing.T) {
	functions := []string{"length", "contains", "keys", "map_get", "map_contains_key", "map_values", "upper", "lower"}
	c := NewShellCompletion(context.Background(), &fakeDataSource{}, functions)

	got := doString(c, "SELECT map_g")
	if !contains(got, "map_get(") {
		t.Errorf("custom function: got %v, want to contain %q", got, "map_get(")
	}

	got = doString(c, "SELECT upp")
	if !contains(got, "upper(") {
		t.Errorf("built-in function: got %v, want to contain %q", got, "upper(")
	}

	// Case-insensitive match: uppercase prefix "MAP_C" still matches "map_contains_key(".
	suffixes, length := c.Do([]rune("SELECT MAP_C"), len("SELECT MAP_C"))
	if length != len("MAP_C") {
		t.Fatalf("expected matched length %d, got %d", len("MAP_C"), length)
	}
	found := false
	for _, s := range suffixes {
		if "map_c"+string(s) == "map_contains_key(" {
			found = true
		}
	}
	if !found {
		t.Errorf("uppercase prefix MAP_C: got suffixes %v, want one completing to %q", suffixes, "map_contains_key(")
	}
}

func TestComplete_FunctionNamesMixWithColumnsAndKeywords(t *testing.T) {
	functions := []string{"length"}
	columns := map[string][]string{"pods": {"labels", "name"}}
	c := NewShellCompletion(context.Background(), &fakeDataSource{columns: columns}, functions)

	got := doCursor(c, "SELECT l| FROM pods")
	if !contains(got, "length(") {
		t.Errorf("expected function 'length(' in candidates: %v", got)
	}
	if !contains(got, "labels") {
		t.Errorf("expected column 'labels' in candidates: %v", got)
	}
	if !contains(got, "like") && !contains(got, "limit") {
		t.Errorf("expected keyword 'like' or 'limit' in candidates: %v", got)
	}
}

func TestComplete_TableAfterFrom(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{tables: []string{"pods", "podtemplates", "nodes"}}, nil)

	got := doString(c, "SELECT name FROM po")
	if !contains(got, "pods") || !contains(got, "podtemplates") {
		t.Errorf("table completion: got %v, want pods + podtemplates", got)
	}
	if contains(got, "nodes") {
		t.Errorf("table completion should filter by prefix 'po': %v", got)
	}
}

func TestComplete_TableAfterDescribeTable(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{tables: []string{"pods", "podtemplates", "nodes"}}, nil)

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

	c := NewShellCompletion(context.Background(), &fakeDataSource{columns: columns}, nil)

	got := doCursor(c, "SELECT sta| FROM pods")

	assert.Contains(t, got, "status", "expected 'status' column in completion candidates")
	assert.NotContains(t, got, "name", "unexpected 'name' column in completion candidates")
}

func TestComplete_UnknownTableNoColumns(t *testing.T) {
	c := NewShellCompletion(context.Background(), &fakeDataSource{columns: map[string][]string{}}, nil)

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

func TestComplete_StructFieldAfterArrow(t *testing.T) {
	fields := map[string][]schema.Field{
		"pods": {
			{Name: "name"},
			{Name: "namespace"},
			{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
				{Name: "containers", Type: schema.FieldTypeList},
				{Name: "nodeName"},
			}},
			{Name: "status", Type: schema.FieldTypeObject, SubFields: []schema.Field{
				{Name: "phase"},
			}},
		},
	}
	c := NewShellCompletion(context.Background(), &fakeDataSource{fields: fields}, nil)

	got := doCursor(c, "SELECT spec->con| FROM pods")
	assert.Contains(t, got, "containers", "expected 'containers' subfield suggestion for spec->con")
	assert.NotContains(t, got, "nodeName", "should filter subfields by typed prefix")
	assert.NotContains(t, got, "status", "should not suggest top-level columns after ->")

	got = doCursor(c, "SELECT spec->| FROM pods")
	assert.Contains(t, got, "containers", "empty word after -> should list all subfields")
	assert.Contains(t, got, "nodeName")

	// Unknown parent path -> no suggestions, no panic.
	got = doCursor(c, "SELECT bogus->fie| FROM pods")
	assert.Empty(t, got, "unresolvable struct path should yield no suggestions")
}

func TestComplete_NestedStructFieldAfterArrow(t *testing.T) {
	fields := map[string][]schema.Field{
		"pods": {
			{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
				{Name: "containers", Type: schema.FieldTypeList, SubFields: []schema.Field{
					{Name: "name"},
					{Name: "image"},
				}},
			}},
		},
	}
	c := NewShellCompletion(context.Background(), &fakeDataSource{fields: fields}, nil)

	got := doCursor(c, "SELECT spec->containers->0->na| FROM pods")
	assert.Contains(t, got, "name", "array index segment should pass through to element subfields")
}

func TestComplete_ColumnCaching(t *testing.T) {
	src := &fakeDataSource{columns: map[string][]string{"pods": {"status"}}}
	c := NewShellCompletion(context.Background(), src, nil)

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
