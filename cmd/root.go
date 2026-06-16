// Package cmd contains the cobra CLI commands for kubectl-sql.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	zaplog "github.com/ebuildy/kubectl-sql/internal/adapter/logger/zap"
	commandQuery "github.com/ebuildy/kubectl-sql/internal/domain/commands/query"
	commandRepl "github.com/ebuildy/kubectl-sql/internal/domain/commands/repl"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	"github.com/ebuildy/kubectl-sql/internal/utils"
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
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		verbosity, _ := cmd.Flags().GetCount("verbose")
		noColor, _ := cmd.Flags().GetBool("no-color")
		l := zaplog.New(logger.Options{Verbosity: verbosity, NoColor: noColor})
		cmd.SetContext(logger.IntoContext(cmd.Context(), l))
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() { _ = logger.FromContext(cmd.Context()).Sync() }()

		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
		kubeContext, _ := cmd.Flags().GetString("context")
		namespace, _ := cmd.Flags().GetString("namespace")
		replFlag, _ := cmd.Flags().GetBool("repl")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		outputFormat, _ := cmd.Flags().GetString("output")
		noColor, _ := cmd.Flags().GetBool("no-color")
		disableBeauty, _ := cmd.Flags().GetBool("disable-beauty")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		watch, _ := cmd.Flags().GetBool("watch")
		yes, _ := cmd.Flags().GetBool("yes")

		config := api.Config{
			Kubeconfig:    kubeconfig,
			KubeContext:   kubeContext,
			Namespace:     namespace,
			PageSize:      pageSize,
			Output:        outputFormat,
			Timeout:       timeout,
			NoColor:       noColor,
			DisableBeauty: disableBeauty,
			DryRun:        dryRun,
			Watch:         watch,
			Yes:           yes,
			Out:           os.Stdout,
		}

		queryCommand, err := commandQuery.NewQueryCommand(cmd.Context(), config)
		if err != nil {
			return fmt.Errorf("kubectl-sql: create query command: %w", err)
		}

		if len(args) == 0 {
			// No positional query: open the REPL. On a TTY (or with --repl)
			// this is the interactive prompt; with piped stdin it reads
			// queries line-by-line in batch mode.
			interactive := replFlag || utils.StdinIsTTY()

			replCommand, err := commandRepl.NewReplCommand(cmd.Context(), config)
			if err != nil {
				return fmt.Errorf("kubectl-sql: create REPL command: %w", err)
			}

			return replCommand.Run(cmd.Context(), interactive, version, projectURL)
		}
		return queryCommand.Run(cmd.Context(), args[0])
	},
}

// Execute runs the root command and exits on error.
// os.Exit is called unconditionally to terminate background goroutines
// spawned by octosql's ristretto caches which have no cleanup path.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var ec api.ExitError
		if errors.As(err, &ec) {
			os.Exit(ec.Code)
		}
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
	rootCmd.PersistentFlags().Bool("disable-beauty", false, "Render struct values as compact single-line JSON without pretty-printing or key colors")
	rootCmd.PersistentFlags().Bool("explain", false, "Print execution plan without running query")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Validate SQL without hitting the API")
	rootCmd.PersistentFlags().BoolP("watch", "w", false, "Stream live resource changes via the Kubernetes WATCH API")
	rootCmd.PersistentFlags().BoolP("yes", "y", false, "Skip the DELETE confirmation prompt (required for non-interactive DELETE)")
	rootCmd.PersistentFlags().BoolP("repl", "i", false, "Open an interactive SQL REPL (default when no query is given)")
	rootCmd.PersistentFlags().CountP("verbose", "v", "Increase log verbosity: -v=info, -vv=debug, -vvv=trace (default error)")
}
