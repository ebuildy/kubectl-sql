package octosql

import (
	"regexp"
	"strings"

	"github.com/cube2222/octosql/parser/sqlparser"
)

// rewriteQuery performs two rewrites on the raw SQL string before parsing:
//  1. Replaces multi-part dotted field references (e.g. metadata.labels.app)
//     with arrow equivalents (metadata->labels->app), or underscore flat names
//     for array-index paths, so octosql's parser handles them correctly.
//  2. Prefixes bare table names in FROM/JOIN clauses with "k8s." so octosql
//     routes them to the KubernetesDatabase.
func rewriteQuery(query string) string {
	query = rewriteDottedFields(query)

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

// arrayIndexPathRe matches paths that contain at least one array index [N].
// Requires at least one [N] bracket — pure dotted paths are excluded.
// e.g. spec.volumes[0], spec.volumes[0].configMap, spec.containers[1].name
var arrayIndexPathRe = regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*|\[\d+\])*\[\d+\](?:\.[a-zA-Z_][a-zA-Z0-9_]*|\[\d+\])*`)

// rewriteDottedFields rewrites field path notation:
//   - Pure dotted paths (no array indices): metadata.labels.app → metadata->labels->app
//   - Paths with array indices: spec.volumes[0].configMap → spec_volumes_0_configMap
//   - Wildcard suffix stripped first: metadata.labels.* → metadata->labels
//
// k8s.pods style table qualifiers are left untouched.
func rewriteDottedFields(query string) string {
	// Pass 1: paths with array indices → underscore flat names (must run before arrow rewrite)
	query = arrayIndexPathRe.ReplaceAllStringFunc(query, func(match string) string {
		if strings.HasPrefix(match, "k8s.") || !strings.ContainsAny(match, "[") {
			return match
		}
		// spec.volumes[0].configMap → spec_volumes_0_configMap
		s := strings.ReplaceAll(match, "[", "_")
		s = strings.ReplaceAll(s, "]", "")
		s = strings.ReplaceAll(s, ".", "_")
		return s
	})
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
	return query
}
