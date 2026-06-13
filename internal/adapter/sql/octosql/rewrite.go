package octosql

import (
	"github.com/cube2222/octosql/parser/sqlparser"
)

// rewriteStatement prefixes bare table names in FROM/JOIN clauses with "k8s."
// so octosql routes them to the KubernetesDatabase. The statement is mutated
// in place and returned for convenience.
//
// This mutates the AST directly rather than re-serialising it with
// sqlparser.String and re-parsing: octosql's BinaryExpr.Format renders the
// "[]" array-index operator as "expr [] idx" (e.g. "containers [] 0"), which
// is not valid syntax and fails to re-parse for queries using index access
// such as spec->containers[0].
func rewriteStatement(stmt sqlparser.Statement) sqlparser.Statement {
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
	return stmt
}
