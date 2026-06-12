package octosql

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cube2222/octosql/parser/sqlparser"
)

// rewriteQuery performs two rewrites on the raw SQL string before parsing:
//  1. Replaces multi-part dotted field references (e.g. metadata.labels.app)
//     with arrow equivalents (metadata->labels->app), and rewrites array-index
//     access (spec->volumes[0]) to array_get() calls, so octosql's parser
//     handles them correctly.
//  2. Prefixes bare table names in FROM/JOIN clauses with "k8s." so octosql
//     routes them to the KubernetesDatabase.
func rewriteQuery(query string) string {
	// @TODO: this is a bit hacky and may have edge cases. A more robust solution would be to implement this as a custom sqlparser.SQLNode that performs the rewrites during parsing, but that requires more invasive changes to the parser and is more work. This regex-based approach should be sufficient for most queries and is easier to implement for now.
	// Must run before sqlparser.Parse: sqlparser accepts field['key'] syntax but
	// cannot round-trip it through String(), so it must be rewritten to
	// map_get(field, 'key') first.
	//query = rewriteDottedFields(query)

	stmt, err := sqlparser.Parse(query)
	if err != nil {
		return query
	}
	sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) { //nolint:errcheck
		switch n := node.(type) {
		case *sqlparser.AliasedTableExpr:
			if tbl, ok := n.Expr.(sqlparser.TableName); ok {
				if tbl.Qualifier.IsEmpty() {
					tbl.Qualifier = sqlparser.NewTableIdent("k8s")
					n.Expr = tbl
				}
			}
		}
		return true, nil
	}, stmt)
	return sqlparser.String(stmt)
}

// dottedWildcardRe matches dotted paths ending in .* (e.g. metadata.labels.*).
var dottedWildcardRe = regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*(?:(?:\.[a-zA-Z_][a-zA-Z0-9_]*)+|\[\d+\](?:\.[a-zA-Z_][a-zA-Z0-9_]*)*))\.\*`)

// dottedFieldRe matches dotted paths that contain NO array indices.
var dottedFieldRe = regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)(\.[a-zA-Z_][a-zA-Z0-9_]*)+\b`)

// arrowIndexRe matches a struct field access chain followed by a numeric index,
// e.g. spec->volumes[0] or spec->containers[1]->name (group 1 = "spec->containers",
// group 2 = "1"). Requires at least one "->". octosql's sqlparser cannot
// round-trip the "[]" indexing operator through String(), so it is rewritten to
// a call to the array_get() function instead, which round-trips like any other
// function call.
var arrowIndexRe = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:->[A-Za-z_][A-Za-z0-9_]*)+)\[(\d+)\]`)

// mapKeyAccessRe matches a (dotted) field path followed by a quoted bracket key,
// e.g. labels['app'] or metadata.labels["app"]. Group 1 is the field path, group 2
// the key. This is map access: the path resolves to a map column whose per-row key
// is looked up — distinct from struct field access (->) and numeric array index.
var mapKeyAccessRe = regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)\[\s*['"]([^'"]*)['"]\s*\]`)

// stringLiteralRe matches single- or double-quoted SQL string literals (no
// embedded escaped quotes), e.g. 'config.json' or "app". Used to protect literal
// contents from the dotted-field rewrites below — a key like 'config.json' must
// not be rewritten to 'config->json'.
var stringLiteralRe = regexp.MustCompile(`'[^']*'|"[^"]*"`)

// rewriteDottedFields rewrites field path notation:
//   - Pure dotted paths (no array indices): metadata.labels.app → metadata->labels->app
//   - Struct field access followed by an array index: spec->volumes[0] → array_get(spec->volumes, 0)
//   - Wildcard suffix stripped first: metadata.labels.* → metadata->labels
//
// k8s.pods style table qualifiers are left untouched. String literals (e.g.
// 'config.json') are protected from these rewrites.
func rewriteDottedFields(query string) string {
	// Pass 0: map key access path['key'] → map_get(path, 'key'). The path's dots are
	// converted to arrows so a nested map column (metadata.labels) resolves as a
	// struct field first. Runs before the array/arrow passes so the inner path is
	// rewritten consistently. Skips k8s.* table qualifiers. Must run before string
	// literal protection: it needs the quotes around the bracket key to match.
	query = mapKeyAccessRe.ReplaceAllStringFunc(query, func(match string) string {
		m := mapKeyAccessRe.FindStringSubmatch(match)
		path, key := m[1], m[2]
		if strings.HasPrefix(path, "k8s.") {
			return match
		}
		arrowPath := strings.ReplaceAll(path, ".", "->")
		return "map_get(" + arrowPath + ", '" + key + "')"
	})

	// Protect string literal contents (e.g. 'config.json') from the dotted-field
	// regexes below by replacing them with placeholders, restored at the end.
	var literals []string
	query = stringLiteralRe.ReplaceAllStringFunc(query, func(match string) string {
		literals = append(literals, match)
		return fmt.Sprintf("\x00%d\x00", len(literals)-1)
	})

	// Pass 1: struct field access followed by a numeric index, e.g.
	// spec->volumes[0] → array_get(spec->volumes, 0). Must run before Pass 3
	// (arrow rewrite) since it matches on the "->" form directly.
	query = arrowIndexRe.ReplaceAllString(query, "array_get($1, $2)")

	// Pass 2: strip wildcard suffix (metadata.labels.* → metadata.labels)
	query = dottedWildcardRe.ReplaceAllStringFunc(query, func(match string) string {
		return match[:len(match)-2] // strip .*
	})
	// Pass 3: pure dotted paths → arrow chains, skip k8s.* table qualifiers
	query = dottedFieldRe.ReplaceAllStringFunc(query, func(match string) string {
		if strings.HasPrefix(match, "k8s.") {
			return match
		}
		return strings.ReplaceAll(match, ".", "->")
	})

	// Restore protected string literals.
	for i, lit := range literals {
		query = strings.ReplaceAll(query, fmt.Sprintf("\x00%d\x00", i), lit)
	}
	return query
}
