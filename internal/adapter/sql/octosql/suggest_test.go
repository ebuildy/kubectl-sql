package octosql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	spellcheckerAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/spellchecker"
	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	"github.com/ebuildy/kubectl-sql/internal/port/spellchecker"
	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

func realSpellChecker() spellchecker.SpellChecker { return spellcheckerAdapter.New() }

// fakeSpellChecker resolves a typo to a configured correction, but only when
// that correction is actually present in the candidate set it is handed. This
// isolates the engine's detection + candidate-scoping logic from the real
// similarity scoring: a test asserting nested-field scoping fails if the engine
// passes the wrong candidate set.
type fakeSpellChecker struct {
	matches map[string]string
}

func (f fakeSpellChecker) ClosestMatch(target string, candidates []string) (string, bool) {
	want, ok := f.matches[target]
	if !ok {
		return "", false
	}
	for _, c := range candidates {
		if c == want {
			return want, true
		}
	}
	return "", false
}

// suggestFakeDS serves a pods-like schema for suggestion tests and resolves a
// small set of queryable resources.
type suggestFakeDS struct{}

func (suggestFakeDS) Resolve(_ context.Context, table string) (k8sport.Resource, error) {
	switch table {
	case "pods", "pod", "po", "deployments":
		return k8sport.Resource{Name: "pods", Version: "v1", Namespaced: true}, nil
	}
	return k8sport.Resource{}, fmt.Errorf("unknown resource %q", table)
}

func (suggestFakeDS) Resources(context.Context) ([]k8sport.Resource, error) {
	return []k8sport.Resource{
		{Name: "pods"}, {Name: "deployments"}, {Name: "services"}, {Name: "nodes"},
	}, nil
}

func (suggestFakeDS) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
	return []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "namespace", Type: schema.FieldTypeString},
		{Name: "status", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "phase", Type: schema.FieldTypeString},
			{Name: "reason", Type: schema.FieldTypeString},
		}},
		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "nodeName", Type: schema.FieldTypeString},
			{Name: "containers", Type: schema.FieldTypeList, SubFields: []schema.Field{
				{Name: "image", Type: schema.FieldTypeString},
				{Name: "name", Type: schema.FieldTypeString},
			}},
		}},
	}, nil
}

func (suggestFakeDS) List(_ context.Context, _ k8sport.Resource, _ k8sport.ListOptions, pageFn func([]map[string]any) error) error {
	return pageFn(nil)
}

func (suggestFakeDS) Delete(context.Context, k8sport.Resource, string, string, k8sport.DeleteOptions) error {
	return nil
}

func runForSuggestion(t *testing.T, sc fakeSpellChecker, sql string) (*portsql.SuggestionError, error) {
	t.Helper()
	eng := New(portsql.Config{Output: "csv"}, suggestFakeDS{}, sc)
	var buf strings.Builder
	err := eng.Execute(context.Background(), portsql.Query{SQL: sql}, &buf)
	var se *portsql.SuggestionError
	if errors.As(err, &se) {
		return se, err
	}
	return nil, err
}

func TestSuggest_KeywordTypo(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{"SLECT": "SELECT"}}
	se, _ := runForSuggestion(t, sc, "SLECT name FROM pods")
	if se == nil {
		t.Fatal("expected a suggestion for SLECT")
	}
	if se.Suggestion.Kind != portsql.SuggestionKindKeyword || se.Suggestion.CorrectedSQL != "SELECT name FROM pods" {
		t.Errorf("got %+v", se.Suggestion)
	}
}

func TestSuggest_KeywordClauseTypo(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{"FORM": "FROM"}}
	se, _ := runForSuggestion(t, sc, "SELECT name FORM pods")
	if se == nil || se.Suggestion.CorrectedSQL != "SELECT name FROM pods" {
		t.Fatalf("expected FORM->FROM correction, got %+v", se)
	}
}

func TestSuggest_TableTypo(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{"pdos": "pods"}}
	se, _ := runForSuggestion(t, sc, "SELECT name FROM pdos")
	if se == nil {
		t.Fatal("expected a suggestion for pdos")
	}
	if se.Suggestion.Kind != portsql.SuggestionKindTable || se.Suggestion.CorrectedSQL != "SELECT name FROM pods" {
		t.Errorf("got %+v", se.Suggestion)
	}
}

func TestSuggest_TopLevelFieldTypo(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{"staus": "status"}}
	se, _ := runForSuggestion(t, sc, "SELECT staus FROM pods")
	if se == nil {
		t.Fatal("expected a suggestion for staus")
	}
	if se.Suggestion.Kind != portsql.SuggestionKindField || se.Suggestion.CorrectedSQL != "SELECT status FROM pods" {
		t.Errorf("got %+v", se.Suggestion)
	}
}

func TestSuggest_NestedFieldTypoScopedToParent(t *testing.T) {
	// "phse" must correct to status->phase (a subfield of status), NOT to an
	// unrelated top-level field. The fake only matches if "phase" is in the
	// candidate set handed to it.
	sc := fakeSpellChecker{matches: map[string]string{"phse": "phase"}}
	se, _ := runForSuggestion(t, sc, "SELECT status->phse FROM pods")
	if se == nil {
		t.Fatal("expected a nested-field suggestion for phse")
	}
	if se.Suggestion.CorrectedSQL != "SELECT status->phase FROM pods" {
		t.Errorf("got corrected %q", se.Suggestion.CorrectedSQL)
	}
}

func TestSuggest_DeepNestedListElementField(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{"imagee": "image"}}
	se, _ := runForSuggestion(t, sc, "SELECT spec->containers[0]->imagee FROM pods")
	if se == nil {
		t.Fatal("expected a suggestion for imagee")
	}
	if se.Suggestion.CorrectedSQL != "SELECT spec->containers[0]->image FROM pods" {
		t.Errorf("got corrected %q", se.Suggestion.CorrectedSQL)
	}
}

func TestSuggest_CrossStagePrecedenceKeywordFirst(t *testing.T) {
	// Both a keyword (SLECT) and a table (pdos) typo: the keyword (parse stage)
	// wins and the table typo is left for the next run.
	sc := fakeSpellChecker{matches: map[string]string{"SLECT": "SELECT", "pdos": "pods"}}
	se, _ := runForSuggestion(t, sc, "SLECT name FROM pdos")
	if se == nil {
		t.Fatal("expected a suggestion")
	}
	if se.Suggestion.Kind != portsql.SuggestionKindKeyword || se.Suggestion.CorrectedSQL != "SELECT name FROM pdos" {
		t.Errorf("expected keyword-first correction, got %+v", se.Suggestion)
	}
}

func TestSuggest_TwoFieldTyposOneAtATime(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{"staus": "status", "nme": "name"}}
	se, _ := runForSuggestion(t, sc, "SELECT staus, nme FROM pods")
	if se == nil {
		t.Fatal("expected a suggestion")
	}
	// Only the first typo is corrected; the second remains.
	if !strings.Contains(se.Suggestion.CorrectedSQL, "nme") {
		t.Errorf("expected second typo 'nme' preserved, got %q", se.Suggestion.CorrectedSQL)
	}
	if se.Suggestion.Typo != "staus" {
		t.Errorf("expected first typo corrected, got typo=%q", se.Suggestion.Typo)
	}
}

func TestSuggest_BaseSegmentChainTypoCorrectedFirst(t *testing.T) {
	// Both base (spc) and nested (contenairs) are typos; octosql reports the
	// base as a top-level unknown variable, so spc->spec is corrected first and
	// contenairs is left for the next run.
	sc := fakeSpellChecker{matches: map[string]string{"spc": "spec", "contenairs": "containers"}}
	se, _ := runForSuggestion(t, sc, "SELECT spc->contenairs FROM pods")
	if se == nil {
		t.Fatal("expected a suggestion")
	}
	if se.Suggestion.CorrectedSQL != "SELECT spec->contenairs FROM pods" {
		t.Errorf("expected base-segment corrected first, got %q", se.Suggestion.CorrectedSQL)
	}
}

func TestSuggest_RealAdapterNoFalsePositiveOnIdentifier(t *testing.T) {
	// With the real spellchecker, a valid identifier ("name") preceding a real
	// keyword typo ("FORM") must not be flagged; FORM->FROM should win.
	eng := New(portsql.Config{Output: "csv"}, suggestFakeDS{}, realSpellChecker())
	var buf strings.Builder
	err := eng.Execute(context.Background(), portsql.Query{SQL: "SELECT name FORM pods"}, &buf)
	var se *portsql.SuggestionError
	if !errors.As(err, &se) {
		t.Fatalf("expected a suggestion, got %v", err)
	}
	if se.Suggestion.Kind != portsql.SuggestionKindKeyword || se.Suggestion.Typo != "FORM" {
		t.Errorf("expected FORM keyword correction, got %+v", se.Suggestion)
	}
}

func TestSuggest_DotNotationConvertedToArrow(t *testing.T) {
	// `spec.annotations` is reported as unknown variable 'spec.annotations'; the
	// fix is to use the -> accessor.
	sc := fakeSpellChecker{matches: map[string]string{}}
	se, _ := runForSuggestion(t, sc, "select spec.annotations from pods")
	if se == nil {
		t.Fatal("expected a dot-notation suggestion")
	}
	if se.Suggestion.Kind != portsql.SuggestionKindDotNotation {
		t.Errorf("expected dot-notation kind, got %q", se.Suggestion.Kind)
	}
	if se.Suggestion.CorrectedSQL != "select spec->annotations from pods" {
		t.Errorf("got corrected %q", se.Suggestion.CorrectedSQL)
	}
	if !strings.Contains(se.Suggestion.Hint(), "use -> not .") {
		t.Errorf("hint should remind to use ->, got %q", se.Suggestion.Hint())
	}
}

func TestSuggest_DeepDotNotationConvertedToArrow(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{}}
	se, _ := runForSuggestion(t, sc, "select metadata.labels.app from pods")
	if se == nil || se.Suggestion.Kind != portsql.SuggestionKindDotNotation {
		t.Fatalf("expected dot-notation suggestion, got %+v", se)
	}
	if se.Suggestion.CorrectedSQL != "select metadata->labels->app from pods" {
		t.Errorf("got corrected %q", se.Suggestion.CorrectedSQL)
	}
}

func TestSuggest_UnterminatedDoubleQuote(t *testing.T) {
	// A missing closing quote is a structural parse failure; the fix appends the
	// matching quote. The spellchecker is irrelevant here but must be non-nil for
	// suggestions to be offered.
	sc := fakeSpellChecker{matches: map[string]string{}}
	se, _ := runForSuggestion(t, sc, `select status from po where nam = "toto`)
	if se == nil {
		t.Fatal("expected a suggestion for the unterminated quote")
	}
	if se.Suggestion.Kind != portsql.SuggestionKindSyntax {
		t.Errorf("expected syntax kind, got %q", se.Suggestion.Kind)
	}
	if se.Suggestion.CorrectedSQL != `select status from po where nam = "toto"` {
		t.Errorf("got corrected %q", se.Suggestion.CorrectedSQL)
	}
}

func TestSuggest_UnterminatedSingleQuote(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{}}
	se, _ := runForSuggestion(t, sc, `select status from po where nam = 'toto`)
	if se == nil || se.Suggestion.CorrectedSQL != `select status from po where nam = 'toto'` {
		t.Fatalf("expected single-quote close, got %+v", se)
	}
}

func TestSuggest_BalancedQuotesNoSyntaxSuggestion(t *testing.T) {
	// unterminatedQuote must report nothing when every quote is closed.
	if _, ok := unterminatedQuote(`select * from pods where name = 'x'`); ok {
		t.Error("balanced quotes should not be flagged as unterminated")
	}
	if _, ok := unterminatedQuote(`select * from pods where name = "a" and ns = "b"`); ok {
		t.Error("balanced double quotes should not be flagged as unterminated")
	}
}

func TestSuggest_EscapedQuoteInsideLiteralStillOpen(t *testing.T) {
	// The literal opens with ' and contains an escaped quote, so it remains open.
	q, ok := unterminatedQuote(`select * from pods where name = 'it\'s`)
	if !ok || q != '\'' {
		t.Errorf("expected an open single-quote literal, got %q,%v", q, ok)
	}
}

func TestSuggest_NoMatchPassthrough(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{}} // never matches
	se, err := runForSuggestion(t, sc, "SELECT xyzzy FROM pods")
	if se != nil {
		t.Fatalf("expected no suggestion, got %+v", se.Suggestion)
	}
	if err == nil {
		t.Fatal("expected the original error to be returned")
	}
}

func TestSuggest_RestOfQueryPreserved(t *testing.T) {
	sc := fakeSpellChecker{matches: map[string]string{"staus": "status"}}
	se, _ := runForSuggestion(t, sc, "SELECT staus FROM pods WHERE name = 'x'")
	if se == nil {
		t.Fatal("expected a suggestion")
	}
	if se.Suggestion.CorrectedSQL != "SELECT status FROM pods WHERE name = 'x'" {
		t.Errorf("rest of query not preserved: %q", se.Suggestion.CorrectedSQL)
	}
}

func TestSuggest_QuotedLiteralNotTreatedAsKeyword(t *testing.T) {
	// barewordTokens must skip the quoted literal 'slect'.
	toks := barewordTokens("SELECT name FROM pods WHERE name = 'slect'")
	for _, tok := range toks {
		if tok == "slect" {
			t.Errorf("quoted literal 'slect' should not be a bareword token, got %v", toks)
		}
	}
}
