package octosql

import (
	"github.com/cube2222/octosql/parser/sqlparser"
)

// rewriteQuery prefixes bare table names in FROM/JOIN clauses with "k8s." so
// octosql routes them to the KubernetesDatabase.
func rewriteQuery(query string) string {
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
