// Package cmd contains the cobra CLI commands for kubectl-sql.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cube2222/octosql/aggregates"
	"github.com/cube2222/octosql/execution"
	"github.com/cube2222/octosql/functions"
	"github.com/cube2222/octosql/logical"
	"github.com/cube2222/octosql/optimizer"
	"github.com/cube2222/octosql/outputs/batch"
	"github.com/cube2222/octosql/outputs/formats"
	"github.com/cube2222/octosql/parser"
	"github.com/cube2222/octosql/parser/sqlparser"
	"github.com/cube2222/octosql/physical"
	"github.com/cube2222/octosql/table_valued_functions"
	"github.com/ebuildy/kubectl-sql/internal/executor"
	k8sclient "github.com/ebuildy/kubectl-sql/internal/k8s"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "kubectl-sql [query]",
	Short: "Query Kubernetes resources using SQL syntax",
	Long: `kubectl-sql is a kubectl plugin that lets you query any Kubernetes resource
using SQL-like syntax for fast debugging, error discovery, and cross-namespace analysis.

Example:
  kubectl sql "SELECT name, namespace, status.phase FROM pods WHERE status.phase != 'Running'"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runQuery(cmd, args[0])
	},
}

// Execute runs the root command and exits on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringP("output", "o", "table", "Output format: table, json, yaml, csv")
	rootCmd.PersistentFlags().String("context", "", "kubeconfig context to use")
	rootCmd.PersistentFlags().StringP("namespace", "n", "", "Restrict query to a single namespace")
	rootCmd.PersistentFlags().String("kubeconfig", "", "Path to kubeconfig (default: ~/.kube/config)")
	rootCmd.PersistentFlags().Int("page-size", 500, "Kubernetes LIST page size")
	rootCmd.PersistentFlags().Duration("timeout", 30*time.Second, "Per-request timeout")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable ANSI colors")
	rootCmd.PersistentFlags().Bool("explain", false, "Print execution plan without running query")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Validate SQL without hitting the API")
}

func runQuery(cmd *cobra.Command, query string) error {
	kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
	kubeContext, _ := cmd.Flags().GetString("context")
	namespace, _ := cmd.Flags().GetString("namespace")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	dynClient, mapper, err := k8sclient.NewDynamicClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
	}

	db := executor.NewKubernetesDatabase(dynClient, mapper, namespace, pageSize)

	env := physical.Environment{
		Aggregates: aggregates.Aggregates,
		Functions:  functions.FunctionMap(),
		Datasources: &physical.DatasourceRepository{
			Databases: map[string]func() (physical.Database, error){
				"k8s": func() (physical.Database, error) { return db, nil },
			},
			FileHandlers: map[string]func(context.Context, string, map[string]string) (physical.DatasourceImplementation, physical.Schema, error){},
		},
		VariableContext: nil,
	}

	// Rewrite bare table names (e.g. FROM pods) to k8s.pods so our DB is used.
	rewritten := rewriteQuery(query)

	statement, err := sqlparser.Parse(rewritten)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return fmt.Errorf("kubectl-sql: parse query: %w", err)
	}
	selectStmt, ok := statement.(sqlparser.SelectStatement)
	if !ok {
		return fmt.Errorf("kubectl-sql: only SELECT statements are supported")
	}

	logicalPlan, outputOptions, err := parser.ParseNode(selectStmt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return fmt.Errorf("kubectl-sql: parse node: %w", err)
	}

	tableValuedFuncs := map[string]logical.TableValuedFunctionDescription{
		"max_diff_watermark": table_valued_functions.MaxDiffWatermark,
		"tumble":             table_valued_functions.Tumble,
		"range":              table_valued_functions.Range,
		"poll":               table_valued_functions.Poll,
	}
	uniqueNameGen := map[string]int{}
	logicalEnv := logical.Environment{
		CommonTableExpressions: map[string]logical.CommonTableExpression{},
		TableValuedFunctions:   tableValuedFuncs,
		UniqueNameGenerator:    uniqueNameGen,
	}

	physicalPlan, mapping, err := typecheckNode(ctx, logicalPlan, env, logicalEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return fmt.Errorf("kubectl-sql: typecheck: %w", err)
	}

	physicalPlan = optimizer.Optimize(physicalPlan)

	// Typecheck ORDER BY expressions.
	orderByExprs := make([]physical.Expression, len(outputOptions.OrderByExpressions))
	for i, expr := range outputOptions.OrderByExpressions {
		pe, tcErr := typecheckExpr(ctx, expr, env.WithRecordSchema(physicalPlan.Schema), logical.Environment{
			CommonTableExpressions: map[string]logical.CommonTableExpression{},
			TableValuedFunctions:   tableValuedFuncs,
			UniqueVariableNames:    &logical.VariableMapping{Mapping: mapping},
			UniqueNameGenerator:    uniqueNameGen,
		})
		if tcErr != nil {
			return fmt.Errorf("kubectl-sql: typecheck order by: %w", tcErr)
		}
		orderByExprs[i] = pe
	}

	var limitInt *int64
	if outputOptions.Limit != nil {
		pe, tcErr := typecheckExpr(ctx, *outputOptions.Limit, env.WithRecordSchema(physicalPlan.Schema), logical.Environment{
			CommonTableExpressions: map[string]logical.CommonTableExpression{},
			TableValuedFunctions:   tableValuedFuncs,
			UniqueVariableNames:    &logical.VariableMapping{Mapping: mapping},
			UniqueNameGenerator:    uniqueNameGen,
		})
		if tcErr != nil {
			return fmt.Errorf("kubectl-sql: typecheck limit: %w", tcErr)
		}
		ee, mErr := pe.Materialize(ctx, env.WithRecordSchema(physicalPlan.Schema))
		if mErr != nil {
			return fmt.Errorf("kubectl-sql: materialize limit: %w", mErr)
		}
		val, evalErr := ee.Evaluate(execution.ExecutionContext{Context: ctx})
		if evalErr != nil {
			return fmt.Errorf("kubectl-sql: evaluate limit: %w", evalErr)
		}
		limitInt = &val.Int
	}

	execPlan, err := physicalPlan.Materialize(ctx, env)
	if err != nil {
		return fmt.Errorf("kubectl-sql: materialize: %w", err)
	}

	execOrderBy := make([]execution.Expression, len(orderByExprs))
	for i, pe := range orderByExprs {
		ee, mErr := pe.Materialize(ctx, env.WithRecordSchema(physicalPlan.Schema))
		if mErr != nil {
			return fmt.Errorf("kubectl-sql: materialize order by: %w", mErr)
		}
		execOrderBy[i] = ee
	}

	reverseMapping := logical.ReverseMapping(mapping)
	outFields := make([]physical.SchemaField, len(physicalPlan.Schema.Fields))
	copy(outFields, physicalPlan.Schema.Fields)
	for i := range outFields {
		outFields[i].Name = reverseMapping[outFields[i].Name]
	}
	outSchema := physical.Schema{Fields: outFields, TimeField: physicalPlan.Schema.TimeField}

	sink := batch.NewOutputPrinter(
		execPlan,
		execOrderBy,
		logical.DirectionsToMultipliers(outputOptions.OrderByDirections),
		limitInt,
		physicalPlan.Schema.NoRetractions,
		outSchema,
		func(w io.Writer) batch.Format {
			return formats.NewTableFormatter(w)
		},
		false,
	)

	return sink.Run(execution.ExecutionContext{Context: ctx, VariableContext: nil})
}

// rewriteQuery prefixes bare table names in FROM/JOIN clauses with "k8s."
// so octosql routes them to the KubernetesDatabase.
// e.g. "SELECT * FROM pods" → "SELECT * FROM k8s.pods"
func rewriteQuery(query string) string {
	// Use sqlparser to walk the AST and prefix all table names.
	stmt, err := sqlparser.Parse(query)
	if err != nil {
		return query // let the caller handle parse errors
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

func typecheckNode(ctx context.Context, node logical.Node, env physical.Environment, logicalEnv logical.Environment) (_ physical.Node, _ map[string]string, outErr error) {
	defer func() {
		if r := recover(); r != nil {
			outErr = fmt.Errorf("typecheck error: %v", r)
		}
	}()
	physicalNode, mapping := node.Typecheck(ctx, env, logicalEnv)
	return physicalNode, mapping, nil
}

func typecheckExpr(ctx context.Context, expr logical.Expression, env physical.Environment, logicalEnv logical.Environment) (_ physical.Expression, outErr error) {
	defer func() {
		if r := recover(); r != nil {
			outErr = fmt.Errorf("typecheck error: %v", r)
		}
	}()
	return expr.Typecheck(ctx, env, logicalEnv), nil
}
