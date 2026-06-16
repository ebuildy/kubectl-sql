package octosql

import (
	"context"
	"regexp"
	"strings"

	"github.com/cube2222/octosql/parser/sqlparser"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// This file implements single-token typo-correction. When a query fails because
// of one mistyped keyword (parse failure), table name, or field (typecheck
// failure) for which a close valid alternative exists, the engine returns a
// portsql.SuggestionError instead of the bare error. Detection keys off each
// stage's authoritative error rather than re-walking the octosql AST, so the
// valid-token set is the one that stage already knows. Coupling to octosql's
// error/panic strings is isolated to the small matchers below and unit-tested.

// sqlKeywordCandidates is the single-word SQL keyword set used as the candidate
// pool when correcting a mistyped keyword. It mirrors the keywords offered by
// Tab completion (internal/adapter/shell/completion, sqlKeywords), reduced to
// single tokens since keyword typo matching is per-token.
var sqlKeywordCandidates = []string{
	"SELECT", "SHOW", "DESCRIBE", "WITH",
	"FROM", "WHERE", "ORDER", "GROUP", "BY", "HAVING", "LIMIT", "OFFSET",
	"DISTINCT", "AS", "AND", "OR", "NOT", "IN", "IS", "NULL", "LIKE", "BETWEEN",
	"COUNT", "ASC", "DESC", "ON", "USING", "UNION", "ALL",
	"JOIN", "INNER", "LEFT", "RIGHT", "FULL", "OUTER", "CROSS",
	"TABLES", "TABLE",
}

// fromTableRe extracts the first table after FROM (optionally k8s.-qualified),
// mirroring the completion source's fromRe.
var fromTableRe = regexp.MustCompile(`(?i)\bfrom\s+(?:k8s\.)?([A-Za-z_][A-Za-z0-9_]*)`)

// resolveResourceRe extracts the resource token from a recovered table-resolution
// failure: `executor: resolve resource "<name>": …`.
var resolveResourceRe = regexp.MustCompile(`resolve resource "([^"]+)"`)

// unknownVariableRe extracts the field token from a top-level unknown-field
// typecheck panic: `unknown variable: '<name>'`.
var unknownVariableRe = regexp.MustCompile(`unknown variable: '([^']+)'`)

// objectFieldAccessRe extracts the field token from a nested-field typecheck
// panic: `object field access of field '<field>' on object expression … without that field`.
var objectFieldAccessRe = regexp.MustCompile(`object field access of field '([^']+)'`)

// bracketIndexRe matches a list-index access (e.g. "[0]") so it can be
// normalised to an arrow segment ("->0") before the chain is split.
var bracketIndexRe = regexp.MustCompile(`\[(\d+)\]`)

// dottedChainRe matches a maximal dotted identifier chain (e.g.
// metadata.labels.app), used to convert dot sub-field access to the -> operator.
var dottedChainRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+`)

// parseSuggestion handles a parse-stage failure: first a structural fix (an
// unterminated string literal), then a mistyped keyword. The structural fix is
// tried first because an open quote makes the rest of the lexing meaningless.
func (e *engine) parseSuggestion(sql string) (portsql.Suggestion, bool) {
	if e.spellchecker == nil {
		return portsql.Suggestion{}, false
	}
	if sug, ok := unterminatedQuoteSuggestion(sql); ok {
		return sug, true
	}
	return e.keywordSuggestion(sql)
}

// unterminatedQuoteSuggestion detects a query that fails to parse because a
// string literal is missing its closing quote and proposes the same query with
// the matching quote appended. It only returns a suggestion when appending the
// quote actually makes the query parse, so it never proposes a fix that doesn't
// help.
func unterminatedQuoteSuggestion(sql string) (portsql.Suggestion, bool) {
	quote, ok := unterminatedQuote(sql)
	if !ok {
		return portsql.Suggestion{}, false
	}
	corrected := sql + string(quote)
	if _, err := sqlparser.Parse(corrected); err != nil {
		return portsql.Suggestion{}, false
	}
	return portsql.Suggestion{
		Kind:         portsql.SuggestionKindSyntax,
		Typo:         string(quote),
		Suggestion:   string(quote),
		CorrectedSQL: corrected,
	}, true
}

// unterminatedQuote returns the opening quote rune (' or ") of an unterminated
// string literal in sql, or false when every quote is closed. Backslash escapes
// inside a literal are honoured so an escaped quote does not close it.
func unterminatedQuote(sql string) (rune, bool) {
	var quote rune
	escaped := false
	for _, r := range sql {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case quote:
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
		}
	}
	return quote, quote != 0
}

// keywordSuggestion scans the raw query's bareword tokens (skipping quoted
// literals) and offers a correction for the first token that is not already an
// exact keyword but whose closest keyword match clears the similarity threshold.
func (e *engine) keywordSuggestion(sql string) (portsql.Suggestion, bool) {
	if e.spellchecker == nil {
		return portsql.Suggestion{}, false
	}
	for _, tok := range barewordTokens(sql) {
		if isExactKeyword(tok) {
			continue
		}
		match, ok := e.spellchecker.ClosestMatch(tok, sqlKeywordCandidates)
		if !ok || strings.EqualFold(match, tok) {
			continue
		}
		return portsql.Suggestion{
			Kind:         portsql.SuggestionKindKeyword,
			Typo:         tok,
			Suggestion:   match,
			CorrectedSQL: replaceFirstWord(sql, tok, match),
		}, true
	}
	return portsql.Suggestion{}, false
}

// typecheckSuggestion dispatches a recovered typecheck error to the table or
// field matcher, in that precedence order.
func (e *engine) typecheckSuggestion(ctx context.Context, sql, errMsg string) (portsql.Suggestion, bool) {
	if e.spellchecker == nil {
		return portsql.Suggestion{}, false
	}
	if sug, ok := e.tableSuggestion(ctx, sql, errMsg); ok {
		return sug, true
	}
	return e.fieldSuggestion(ctx, sql, errMsg)
}

// tableSuggestion offers a correction for a mistyped resource name, with the
// queryable resource names as the candidate set.
func (e *engine) tableSuggestion(ctx context.Context, sql, errMsg string) (portsql.Suggestion, bool) {
	m := resolveResourceRe.FindStringSubmatch(errMsg)
	if m == nil {
		return portsql.Suggestion{}, false
	}
	typo := m[1]
	resources, err := e.ds.Resources(ctx)
	if err != nil {
		return portsql.Suggestion{}, false
	}
	names := make([]string, 0, len(resources))
	for _, r := range resources {
		names = append(names, r.Name)
	}
	match, ok := e.spellchecker.ClosestMatch(typo, names)
	if !ok || strings.EqualFold(match, typo) {
		return portsql.Suggestion{}, false
	}
	return portsql.Suggestion{
		Kind:         portsql.SuggestionKindTable,
		Typo:         typo,
		Suggestion:   match,
		CorrectedSQL: replaceFirstWord(sql, typo, match),
	}, true
}

// fieldSuggestion offers a correction for a mistyped field. Candidates are
// scoped to the failing position: a top-level unknown variable uses the table's
// top-level fields; a nested object-field-access uses the subfields of the
// parent resolved by walking the -> chain preceding the failing segment.
func (e *engine) fieldSuggestion(ctx context.Context, sql, errMsg string) (portsql.Suggestion, bool) {
	// Dotted sub-field access (e.g. spec.annotations) is reported as an unknown
	// variable whose name contains a dot. The fix is structural — switch the dot
	// accessor(s) to the -> operator — so it is handled before (and independently
	// of) similarity-based field correction and table resolution.
	if m := unknownVariableRe.FindStringSubmatch(errMsg); m != nil && strings.Contains(m[1], ".") {
		return dotNotationSuggestion(sql, m[1])
	}

	table := tableInQuery(sql)
	if table == "" {
		return portsql.Suggestion{}, false
	}
	topFields, ok := e.tableTopFields(ctx, table)
	if !ok {
		return portsql.Suggestion{}, false
	}

	// Top-level unknown field (including the base segment of a -> chain).
	if m := unknownVariableRe.FindStringSubmatch(errMsg); m != nil {
		typo := lastDotSegment(m[1])
		return e.rankField(sql, typo, fieldNames(topFields))
	}

	// Nested field whose parent segments are valid: candidates are the parent
	// struct's subfields.
	if m := objectFieldAccessRe.FindStringSubmatch(errMsg); m != nil {
		typo := m[1]
		parent, ok := parentChainOf(sql, typo)
		if !ok {
			return portsql.Suggestion{}, false
		}
		sub := schema.SubFieldsAtPath(topFields, parent)
		if sub == nil {
			return portsql.Suggestion{}, false
		}
		return e.rankField(sql, typo, fieldNames(sub))
	}

	return portsql.Suggestion{}, false
}

// dotNotationSuggestion converts dotted sub-field access (e.g. spec.annotations)
// to the arrow operator (spec->annotations), reminding the user that object
// sub-fields are accessed with -> rather than ".". octosql may report only the
// trailing segments of the offending reference (e.g. 'labels.app' for
// metadata.labels.app), so the suggestion converts the full contiguous dotted
// chain in the query that contains the reported token — giving a clean one-shot
// fix and avoiding touching unrelated dotted tokens such as file sources
// (notes.json).
func dotNotationSuggestion(sql, name string) (portsql.Suggestion, bool) {
	if !strings.Contains(name, ".") {
		return portsql.Suggestion{}, false
	}
	chain := name
	for _, c := range dottedChainRe.FindAllString(sql, -1) {
		if strings.Contains(c, name) {
			chain = c
			break
		}
	}
	arrow := strings.ReplaceAll(chain, ".", "->")
	return portsql.Suggestion{
		Kind:         portsql.SuggestionKindDotNotation,
		Typo:         chain,
		Suggestion:   arrow,
		CorrectedSQL: replaceFirstWord(sql, chain, arrow),
	}, true
}

// rankField builds a field suggestion when the typo has a close match in
// candidates.
func (e *engine) rankField(sql, typo string, candidates []string) (portsql.Suggestion, bool) {
	match, ok := e.spellchecker.ClosestMatch(typo, candidates)
	if !ok || strings.EqualFold(match, typo) {
		return portsql.Suggestion{}, false
	}
	return portsql.Suggestion{
		Kind:         portsql.SuggestionKindField,
		Typo:         typo,
		Suggestion:   match,
		CorrectedSQL: replaceFirstWord(sql, typo, match),
	}, true
}

// tableTopFields resolves table and returns its inferred top-level fields.
func (e *engine) tableTopFields(ctx context.Context, table string) ([]schema.Field, bool) {
	r, err := e.ds.Resolve(ctx, strings.ToLower(table))
	if err != nil {
		return nil, false
	}
	fields, err := e.ds.InferSchema(ctx, r)
	if err != nil || len(fields) == 0 {
		return nil, false
	}
	return fields, true
}

// parentChainOf finds the -> chain in sql that ends in field and returns the
// chain segments preceding field (the parent path), normalising list indices
// (e.g. spec->containers[0]->imagee yields ["spec","containers","0"]). Returns
// false when no such chain is found.
func parentChainOf(sql, field string) ([]string, bool) {
	re := regexp.MustCompile(`((?:[A-Za-z_][A-Za-z0-9_]*(?:\[\d+\])*)(?:->(?:[A-Za-z_][A-Za-z0-9_]*)(?:\[\d+\])*)*)->` + regexp.QuoteMeta(field) + `\b`)
	m := re.FindStringSubmatch(sql)
	if m == nil {
		return nil, false
	}
	parent := bracketIndexRe.ReplaceAllString(m[1], "->$1")
	return strings.Split(parent, "->"), true
}

// tableInQuery extracts the FROM table from sql, or "" if none.
func tableInQuery(sql string) string {
	m := fromTableRe.FindStringSubmatch(sql)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// barewordTokens returns the identifier tokens of sql that lie outside any
// quoted string literal, in order. Quoted literals (single or double) are
// skipped so a value resembling a keyword is never treated as a typo.
func barewordTokens(sql string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range sql {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch {
		case r == '\'' || r == '"':
			flush()
			quote = r
		case isWordChar(r):
			cur.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// fieldNames extracts the Name of each field.
func fieldNames(fields []schema.Field) []string {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return names
}

func isWordChar(r rune) bool {
	return r == '_' ||
		(r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9')
}

// isExactKeyword reports whether tok is already one of the supported keywords
// (case-insensitive), so it should not be treated as a typo.
func isExactKeyword(tok string) bool {
	up := strings.ToUpper(tok)
	for _, kw := range sqlKeywordCandidates {
		if kw == up {
			return true
		}
	}
	return false
}

// lastDotSegment returns the substring after the last '.', so a qualified
// variable name (e.g. "pods.staus") yields the bare field token ("staus").
func lastDotSegment(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// replaceFirstWord replaces the first whole-word (identifier-bounded) occurrence
// of typo in sql with repl, leaving the rest of the query verbatim.
func replaceFirstWord(sql, typo, repl string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(typo) + `\b`)
	loc := re.FindStringIndex(sql)
	if loc == nil {
		return sql
	}
	return sql[:loc[0]] + repl + sql[loc[1]:]
}
