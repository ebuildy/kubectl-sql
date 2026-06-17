// Package cmd contains the cobra CLI commands for kubectl-sql.
package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	zaplog "github.com/ebuildy/kubectl-sql/internal/adapter/logger/zap"
	"github.com/ebuildy/kubectl-sql/internal/app"
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
		uiFlag, _ := cmd.Flags().GetBool("ui")
		uiAddress, _ := cmd.Flags().GetString("ui-address")
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

		// --ui starts the local web server instead of running a query or REPL.
		// Validate the bind address before contacting the cluster so a bad
		// value fails fast without starting anything.
		if uiFlag {
			if err := validateUIAddress(uiAddress); err != nil {
				return api.ExitError{Code: 1, Err: err}
			}
			warnIfNonLoopback(uiAddress)

			uiCommand, err := app.NewUICommand(cmd.Context(), config, uiAddress)
			if err != nil {
				return err
			}

			initialQuery := ""
			if len(args) > 0 {
				initialQuery = args[0]
			}
			return uiCommand.Run(cmd.Context(), initialQuery)
		}

		queryCommand, err := app.NewQueryCommand(cmd.Context(), config)
		if err != nil {
			return fmt.Errorf("kubectl-sql: create query command: %w", err)
		}

		if len(args) == 0 {
			// No positional query: open the REPL. On a TTY (or with --repl)
			// this is the interactive prompt; with piped stdin it reads
			// queries line-by-line in batch mode.
			interactive := replFlag || utils.StdinIsTTY()

			replCommand, err := app.NewReplCommand(cmd.Context(), config)
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
	rootCmd.PersistentFlags().Bool("ui", false, "Start a local web UI instead of running a query")
	rootCmd.PersistentFlags().String("ui-address", "127.0.0.1:8080", "host:port the web UI binds to (loopback by default)")
	rootCmd.PersistentFlags().CountP("verbose", "v", "Increase log verbosity: -v=info, -vv=debug, -vvv=trace (default error)")
}

// validateUIAddress checks that addr is a valid host:port with a numeric,
// in-range port. It returns a clear error naming the bad value so the command
// can fail fast before contacting the cluster or starting a server.
func validateUIAddress(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("kubectl-sql: invalid --ui-address %q: expected host:port", addr)
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("kubectl-sql: invalid --ui-address %q: port must be a number in 1-65535", addr)
	}
	return nil
}

// warnIfNonLoopback prints a warning to stderr when the UI binds to a
// non-loopback host, since the read-only query API is then reachable from the
// network. addr is assumed already validated.
func warnIfNonLoopback(addr string) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	if isLoopbackHost(host) {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: --ui-address %q binds to a non-loopback address; the query API will be reachable from the network\n", addr)
}

// isLoopbackHost reports whether host is loopback-only. An empty host (bind all
// interfaces) is treated as non-loopback.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
