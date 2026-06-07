// Package cmd contains the cobra CLI commands for kubectl-sql.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	k8sadapter "github.com/ebuildy/kubectl-sql/internal/adapter/datasources/k8s"
	zaplog "github.com/ebuildy/kubectl-sql/internal/adapter/logger/zap"
	octosqladapter "github.com/ebuildy/kubectl-sql/internal/adapter/sql/octosql"
	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
	"github.com/ebuildy/kubectl-sql/internal/repl"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var rootCmd = &cobra.Command{
	Use:   "kubectl-sql [query]",
	Short: "Query Kubernetes resources using SQL syntax",
	Long: `kubectl-sql is a kubectl plugin that lets you query any Kubernetes resource
using SQL-like syntax for fast debugging, error discovery, and cross-namespace analysis.

Example:
  kubectl sql "SELECT name, namespace, status.phase FROM pods WHERE status.phase != 'Running'"`,
	Args: cobra.MaximumNArgs(1),
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		verbosity, _ := cmd.Flags().GetCount("verbose")
		noColor, _ := cmd.Flags().GetBool("no-color")
		l := zaplog.New(logger.Options{Verbosity: verbosity, NoColor: noColor})
		cmd.SetContext(logger.IntoContext(cmd.Context(), l))
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() { _ = logger.FromContext(cmd.Context()).Sync() }()
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
	rootCmd.PersistentFlags().CountP("verbose", "v", "Increase log verbosity: -v=info, -vv=debug (default error)")
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

// cliCompletionSource implements repl.CompletionSource over the k8s DataSource port.
type cliCompletionSource struct {
	ctx context.Context
	ds  k8sport.DataSource
}

// newCompletionSource builds a completion source, returning nil if the cluster
// connection fails (completion is then disabled).
func newCompletionSource(ctx context.Context, kubeconfig, kubeContext, namespace string) repl.CompletionSource {
	ds, err := k8sadapter.New(kubeconfig, kubeContext, namespace)
	if err != nil {
		return nil
	}
	return &cliCompletionSource{ctx: ctx, ds: ds}
}

// Tables returns queryable resource names via the port.
func (s *cliCompletionSource) Tables() []string {
	resources, err := s.ds.Resources(s.ctx)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(resources))
	for _, r := range resources {
		names = append(names, r.Name)
	}
	return names
}

// Columns returns the column names for a table, or nil if it cannot be resolved.
func (s *cliCompletionSource) Columns(table string) []string {
	resource, err := s.ds.Resolve(s.ctx, strings.ToLower(table))
	if err != nil {
		return nil
	}
	fields, err := s.ds.InferSchema(s.ctx, resource)
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
	log := logger.FromContext(ctx)
	start := time.Now()
	log.Info("query accepted", logger.String("query", query), logger.String("namespace", namespace))

	if strings.EqualFold(strings.TrimSpace(query), "show tables") {
		ds, err := k8sadapter.New(kubeconfig, kubeContext, namespace)
		if err != nil {
			return fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
		}
		log.Debug("cluster connection established", logger.String("mode", "show tables"))
		return runShowTables(ctx, ds)
	}

	ds, err := k8sadapter.New(kubeconfig, kubeContext, namespace)
	if err != nil {
		return fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
	}
	log.Info("cluster connection established")

	if tok := strings.Fields(strings.ToLower(strings.TrimSpace(query))); len(tok) >= 2 && tok[0] == "describe" && tok[1] == "table" {
		parts := strings.Fields(strings.TrimSpace(query))
		if len(parts) < 3 {
			fmt.Fprintln(os.Stderr, "usage: DESCRIBE TABLE <resource>")
			return fmt.Errorf("kubectl-sql: missing resource name")
		}
		resource := strings.ToLower(parts[2])
		return runDescribeTable(ctx, ds, resource)
	}

	outputFormat, _ := cmd.Flags().GetString("output")
	noColor, _ := cmd.Flags().GetBool("no-color")
	eng := octosqladapter.New(ds)
	execErr := eng.Execute(ctx, portsql.Query{
		SQL:       query,
		Output:    outputFormat,
		Namespace: namespace,
		PageSize:  pageSize,
		NoColor:   noColor,
	}, w)
	log.Debug("query completed", logger.Duration("elapsed", time.Since(start)))
	return execErr
}

func runDescribeTable(ctx context.Context, ds k8sport.DataSource, resource string) error {
	r, err := ds.Resolve(ctx, resource)
	if err != nil {
		return fmt.Errorf("kubectl-sql: unknown resource %q: %w", resource, err)
	}

	fields, _ := ds.InferSchema(ctx, r)
	if len(fields) == 0 {
		fields = []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "namespace", Type: schema.FieldTypeString},
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

func runShowTables(ctx context.Context, ds k8sport.DataSource) error {
	resources, err := ds.Resources(ctx)
	if err != nil {
		return fmt.Errorf("kubectl-sql: list API resources: %w", err)
	}

	type row struct{ name, aliases, group, version string }
	rows := make([]row, 0, len(resources))
	for _, r := range resources {
		rows = append(rows, row{r.Name, strings.Join(r.Aliases, ","), r.Group, r.Version})
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

	log := logger.FromContext(ctx)
	tick := func() error {
		log.Debug("watch tick: refreshing query")
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
