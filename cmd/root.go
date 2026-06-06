// Package cmd contains the cobra CLI commands for kubectl-sql.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/cube2222/octosql/aggregates"
	"github.com/cube2222/octosql/execution"
	"github.com/cube2222/octosql/functions"
	"github.com/cube2222/octosql/logical"
	"github.com/cube2222/octosql/optimizer"
	"github.com/cube2222/octosql/parser"
	"github.com/cube2222/octosql/parser/sqlparser"
	"github.com/cube2222/octosql/physical"
	"github.com/cube2222/octosql/table_valued_functions"
	"github.com/ebuildy/kubectl-sql/internal/executor"
	k8sclient "github.com/ebuildy/kubectl-sql/internal/k8s"
	"github.com/ebuildy/kubectl-sql/internal/output"
	"github.com/ebuildy/kubectl-sql/internal/repl"
	internalschema "github.com/ebuildy/kubectl-sql/internal/schema"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
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
			replFlag, _ := cmd.Flags().GetBool("repl")
			interactive := replFlag || repl.StdinIsTTY()
			// No positional query: open the REPL. On a TTY (or with --repl)
			// this is the interactive prompt; with piped stdin it reads
			// queries line-by-line in batch mode.
			return runREPL(cmd, interactive)
		}
		return runQuery(cmd, args[0])
	},
}

// Execute runs the root command and exits on error.
// os.Exit is called unconditionally to terminate background goroutines
// spawned by octosql's ristretto caches which have no cleanup path.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
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
	rootCmd.PersistentFlags().BoolP("watch", "w", false, "Stream live resource changes via the Kubernetes WATCH API")
	rootCmd.PersistentFlags().BoolP("repl", "i", false, "Open an interactive SQL REPL (default when no query is given)")
}

// runREPL wires the REPL package to the cobra command, forwarding all flags via
// a closure over runQueryWithWriter. interactive selects the prompt-driven loop;
// when false the REPL reads queries from stdin in batch mode.
func runREPL(cmd *cobra.Command, interactive bool) error {
	kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
	kubeContext, _ := cmd.Flags().GetString("context")
	namespace, _ := cmd.Flags().GetString("namespace")

	runQueryFn := func(ctx context.Context, query string, w io.Writer) error {
		timeout, _ := cmd.Flags().GetDuration("timeout")
		queryCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return runQueryWithWriter(cmd, queryCtx, query, kubeconfig, kubeContext, namespace, w)
	}

	replCfg := repl.Config{
		RunQuery: runQueryFn,
		Stdin:    os.Stdin,
		IsTTY:    interactive,
	}

	// Tab completion only matters for the interactive prompt. Build the source
	// best-effort: if the cluster is unreachable, completion is simply disabled
	// rather than aborting the REPL.
	if interactive {
		if src := newCompletionSource(cmd.Context(), kubeconfig, kubeContext, namespace); src != nil {
			replCfg.Completion = src
		}
	}

	return repl.Run(cmd.Context(), replCfg, os.Stdout)
}

// cliCompletionSource implements repl.CompletionSource over discovery (for
// table names) and the schema inferrer (for column names).
type cliCompletionSource struct {
	ctx      context.Context
	mapper   meta.RESTMapper
	inferrer internalschema.SchemaInferrer
	disco    interface {
		ServerPreferredResources() ([]*metav1.APIResourceList, error)
	}
}

// newCompletionSource builds a completion source, returning nil if the cluster
// connection fails (completion is then disabled).
func newCompletionSource(ctx context.Context, kubeconfig, kubeContext, namespace string) repl.CompletionSource {
	dynClient, mapper, discoClient, err := k8sclient.NewDynamicClient(kubeconfig, kubeContext)
	if err != nil {
		return nil
	}
	inferrer := internalschema.NewCompositeInferrer(
		internalschema.NewOpenAPIInferrer(discoClient),
		internalschema.NewSampleInferrer(dynClient, namespace),
	)
	return &cliCompletionSource{ctx: ctx, mapper: mapper, inferrer: inferrer, disco: discoClient}
}

// Tables returns queryable resource names, using the same filtering as SHOW TABLES.
func (s *cliCompletionSource) Tables() []string {
	lists, err := s.disco.ServerPreferredResources()
	if err != nil {
		return nil
	}
	var names []string
	for _, list := range lists {
		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue // skip subresources like pods/log
			}
			names = append(names, r.Name)
		}
	}
	return names
}

// Columns returns the column names for a table, or nil if it cannot be resolved.
func (s *cliCompletionSource) Columns(table string) []string {
	gvr, err := s.mapper.ResourceFor(k8sschema.GroupVersionResource{Resource: strings.ToLower(table)})
	if err != nil {
		return nil
	}
	fields, err := s.inferrer.InferFields(s.ctx, gvr)
	if err != nil {
		return nil
	}
	cols := make([]string, 0, len(fields))
	for _, f := range fields {
		cols = append(cols, f.Name)
	}
	return cols
}

func runQuery(cmd *cobra.Command, query string) error {
	kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
	kubeContext, _ := cmd.Flags().GetString("context")
	namespace, _ := cmd.Flags().GetString("namespace")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	watch, _ := cmd.Flags().GetBool("watch")

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	if watch {
		return runWatch(cmd, ctx, query, kubeconfig, kubeContext, namespace)
	}

	return runQueryWithWriter(cmd, ctx, query, kubeconfig, kubeContext, namespace, os.Stdout)
}

func runQueryWithWriter(cmd *cobra.Command, ctx context.Context, query, kubeconfig, kubeContext, namespace string, w io.Writer) error {
	pageSize, _ := cmd.Flags().GetInt("page-size")

	if strings.EqualFold(strings.TrimSpace(query), "show tables") {
		_, _, discoClient, err := k8sclient.NewDynamicClient(kubeconfig, kubeContext)
		if err != nil {
			return fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
		}
		return runShowTables(discoClient)
	}

	dynClient, mapper, discoClient, err := k8sclient.NewDynamicClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
	}

	inferrer := internalschema.NewCompositeInferrer(
		internalschema.NewOpenAPIInferrer(discoClient),
		internalschema.NewSampleInferrer(dynClient, namespace),
	)

	if tok := strings.Fields(strings.ToLower(strings.TrimSpace(query))); len(tok) >= 2 && tok[0] == "describe" && tok[1] == "table" {
		parts := strings.Fields(strings.TrimSpace(query))
		if len(parts) < 3 {
			fmt.Fprintln(os.Stderr, "usage: DESCRIBE TABLE <resource>")
			return fmt.Errorf("kubectl-sql: missing resource name")
		}
		resource := strings.ToLower(parts[2])
		return runDescribeTable(ctx, mapper, inferrer, resource)
	}

	db := executor.NewKubernetesDatabase(dynClient, mapper, namespace, pageSize, inferrer)

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

	outputFormat, _ := cmd.Flags().GetString("output")
	return output.Render(
		execution.ExecutionContext{Context: ctx, VariableContext: nil},
		execPlan,
		output.Options{
			Format:          outputFormat,
			Limit:           limitInt,
			OrderBy:         execOrderBy,
			OrderDirections: logical.DirectionsToMultipliers(outputOptions.OrderByDirections),
			Schema:          outSchema,
			Writer:          w,
		},
	)
}

func runDescribeTable(ctx context.Context, mapper meta.RESTMapper, inferrer internalschema.SchemaInferrer, resource string) error {
	gvr, err := mapper.ResourceFor(k8sschema.GroupVersionResource{Resource: resource})
	if err != nil {
		return fmt.Errorf("kubectl-sql: unknown resource %q: %w", resource, err)
	}

	fields, _ := inferrer.InferFields(ctx, gvr)
	if len(fields) == 0 {
		fields = []internalschema.Field{
			{Name: "name", Type: internalschema.FieldTypeString},
			{Name: "namespace", Type: internalschema.FieldTypeString},
		}
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"COLUMN", "TYPE"})
	table.SetAutoFormatHeaders(false)
	for _, f := range fields {
		table.Append([]string{f.Name, string(f.Type)})
	}
	table.Render()
	return nil
}

func runShowTables(discoClient interface {
	ServerPreferredResources() ([]*metav1.APIResourceList, error)
}) error {
	lists, err := discoClient.ServerPreferredResources()
	if err != nil {
		return fmt.Errorf("kubectl-sql: list API resources: %w", err)
	}

	type row struct{ name, aliases, group, version string }
	var rows []row
	for _, list := range lists {
		gv := list.GroupVersion
		var group, version string
		if idx := strings.LastIndex(gv, "/"); idx >= 0 {
			group, version = gv[:idx], gv[idx+1:]
		} else {
			version = gv
		}
		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue // skip subresources like pods/log
			}
			rows = append(rows, row{r.Name, strings.Join(r.ShortNames, ","), group, version})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].name < rows[j].name
	})

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"NAME", "ALIASES", "GROUP", "VERSION"})
	table.SetAutoFormatHeaders(false)
	for _, r := range rows {
		table.Append([]string{r.name, r.aliases, r.group, r.version})
	}
	table.Render()
	return nil
}

// rewriteQuery performs two rewrites on the raw SQL string before parsing:
//  1. Replaces multi-part dotted field references (e.g. metadata.labels.app)
//     with underscore equivalents (metadata_labels_app) so octosql's SQL parser
//     does not misinterpret them as table.column qualifiers.
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

// runWatch polls the query every 5 seconds, clearing the terminal and reprinting
// the full result table on each tick — identical output to a normal batch query.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func runWatch(cmd *cobra.Command, ctx context.Context, query, kubeconfig, kubeContext, namespace string) error {
	const pollInterval = 5 * time.Second

	// Handle SIGINT/SIGTERM: cancel context for a clean exit.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go func() {
		select {
		case <-sigCh:
			watchCancel()
		case <-watchCtx.Done():
		}
	}()

	isTTY := isTerminal(os.Stdout)

	tick := func() error {
		var buf strings.Builder
		if err := runQueryWithWriter(cmd, watchCtx, query, kubeconfig, kubeContext, namespace, &buf); err != nil {
			return err
		}
		if isTTY {
			_, _ = fmt.Fprint(os.Stdout, "\033[H\033[2J")
		}
		_, _ = fmt.Fprint(os.Stdout, buf.String())
		return nil
	}

	// First tick immediately.
	if err := tick(); err != nil {
		return err
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-watchCtx.Done():
			return nil
		case <-ticker.C:
			if err := tick(); err != nil {
				return err
			}
		}
	}
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
