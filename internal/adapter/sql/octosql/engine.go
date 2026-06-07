// Package octosql is the octosql-backed adapter for the SQL-engine port
// (internal/port/sql). It is the ONLY package in the repository that imports
// github.com/cube2222/octosql. It owns the full query pipeline — dot/arrow
// rewrite, parse, plan, typecheck, optimize, execute — and renders the result.
// Cluster data is obtained solely through the injected k8s DataSource port; this
// package imports no client-go.
package octosql

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/cube2222/octosql/aggregates"
	"github.com/cube2222/octosql/execution"
	octofunctions "github.com/cube2222/octosql/functions"
	"github.com/cube2222/octosql/logical"
	"github.com/cube2222/octosql/optimizer"
	"github.com/cube2222/octosql/parser"
	"github.com/cube2222/octosql/parser/sqlparser"
	"github.com/cube2222/octosql/physical"
	"github.com/cube2222/octosql/table_valued_functions"

	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// engine implements the sql.Engine port over octosql.
type engine struct {
	ds     k8sport.DataSource
	env    physical.Environment
	config portsql.Config
}

// New builds an octosql-backed SQL engine that sources data through the given
// k8s DataSource port. The returned value is port-typed.
func New(config portsql.Config, ds k8sport.DataSource) portsql.Engine {
	db := NewKubernetesDatabase(ds, config.Namespace, config.PageSize)

	baseFunctions := octofunctions.FunctionMap()
	for k, v := range FunctionMap() {
		baseFunctions[k] = v
	}

	env := physical.Environment{
		Aggregates: aggregates.Aggregates,
		Functions:  baseFunctions,
		Datasources: &physical.DatasourceRepository{
			Databases: map[string]func() (physical.Database, error){
				"k8s": func() (physical.Database, error) { return db, nil },
			},
			FileHandlers: map[string]func(context.Context, string, map[string]string) (physical.DatasourceImplementation, physical.Schema, error){},
		},
		VariableContext: nil,
	}

	return &engine{ds: ds, env: env, config: config}
}

// Execute runs the query through the full octosql pipeline and writes the
// rendered result to w.
func (e *engine) Execute(ctx context.Context, q portsql.Query, w io.Writer) error {
	log := logger.FromContext(ctx)

	env := e.env

	// Rewrite bare table names (e.g. FROM pods) to k8s.pods so our DB is used.
	rewritten := rewriteQuery(q.SQL)
	log.Debug("query rewritten", logger.String("rewritten", rewritten))

	statement, err := sqlparser.Parse(rewritten)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return fmt.Errorf("octosql: parse query: %w", err)
	}
	log.Debug("query parsed")
	selectStmt, ok := statement.(sqlparser.SelectStatement)
	if !ok {
		return fmt.Errorf("octosql: only SELECT statements are supported")
	}

	logicalPlan, outputOptions, err := parser.ParseNode(selectStmt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return fmt.Errorf("octosql: parse node: %w", err)
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
		return fmt.Errorf("octosql: typecheck: %w", err)
	}

	log.Debug("query typechecked")
	physicalPlan = optimizer.Optimize(physicalPlan)
	log.Debug("query optimized")

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
			return fmt.Errorf("octosql: typecheck order by: %w", tcErr)
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
			return fmt.Errorf("octosql: typecheck limit: %w", tcErr)
		}
		ee, mErr := pe.Materialize(ctx, env.WithRecordSchema(physicalPlan.Schema))
		if mErr != nil {
			return fmt.Errorf("octosql: materialize limit: %w", mErr)
		}
		val, evalErr := ee.Evaluate(execution.ExecutionContext{Context: ctx})
		if evalErr != nil {
			return fmt.Errorf("octosql: evaluate limit: %w", evalErr)
		}
		limitInt = &val.Int
	}

	execPlan, err := physicalPlan.Materialize(ctx, env)
	if err != nil {
		return fmt.Errorf("octosql: materialize: %w", err)
	}

	execOrderBy := make([]execution.Expression, len(orderByExprs))
	for i, pe := range orderByExprs {
		ee, mErr := pe.Materialize(ctx, env.WithRecordSchema(physicalPlan.Schema))
		if mErr != nil {
			return fmt.Errorf("octosql: materialize order by: %w", mErr)
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

	return Render(
		execution.ExecutionContext{Context: ctx, VariableContext: nil},
		execPlan,
		Options{
			Format:          e.config.Output,
			Limit:           limitInt,
			OrderBy:         execOrderBy,
			OrderDirections: logical.DirectionsToMultipliers(outputOptions.OrderByDirections),
			Schema:          outSchema,
			Writer:          w,
		},
	)
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
