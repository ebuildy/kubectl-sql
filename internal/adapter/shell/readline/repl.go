// Package repl implements the interactive Read-Eval-Print-Loop for kubectl-sql.
//
// When kubectl-sql is invoked with no positional query argument it falls into
// this loop: a "sql> " prompt reads a query, executes it via the injected
// RunQuery function, prints the result, and loops until the user quits.
//
// The package has no dependency on cmd/ — the query runner is passed in as a
// function value (Config.RunQuery) to avoid an import cycle.
package readline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chzyer/readline"

	"github.com/ebuildy/kubectl-sql/internal/port/logger"

	shellCompletionPort "github.com/ebuildy/kubectl-sql/internal/port/autocomplete"
)

const prompt = "sql> "

// RunQueryFunc executes a single SQL query, writing rendered output to w.
// It mirrors the signature of cmd.runQueryWithWriter minus the cobra command,
// which is captured in the closure passed by the caller.
type RunQueryFunc func(ctx context.Context, query string, w io.Writer) error

// Config carries everything the REPL needs that originates from CLI flags.
// All query-execution concerns are encapsulated in RunQuery, so the REPL stays
// agnostic of kubeconfig/context/namespace/output details.
type NewReadlineShell struct {
	// RunQuery executes one query against the cluster and renders the result.
	RunQuery RunQueryFunc
	//
	IOIn io.Reader
	//
	IOOut io.Writer
	// IsTTY reports whether interactive mode should be used. When false, the
	// REPL reads queries line-by-line without a prompt (batch mode).
	IsTTY bool
	// Completion, when non-nil, enables Tab autocomplete in interactive mode
	// for SQL keywords, table names, and column names.
	Completion shellCompletionPort.ShellCompletionRunner
}

// Run starts the REPL. If the input is not a TTY it falls back to batch mode
// (line-by-line stdin, no prompt). Returns nil on a clean exit (\q, EOF, SIGINT).
func (s *NewReadlineShell) Run(ctx context.Context) error {
	if s.RunQuery == nil {
		return fmt.Errorf("repl: RunQuery is required")
	}
	if !s.IsTTY {
		logger.FromContext(ctx).Info("repl started", logger.String("mode", "batch"))
		return s.runBatch(ctx)
	}
	logger.FromContext(ctx).Info("repl started", logger.String("mode", "interactive"))
	return s.runInteractive(ctx)
}

// runBatch reads queries from stdin one line at a time, executing each. Empty
// lines are skipped. It stops on EOF and returns nil. Per-query errors are
// printed to stderr but do not abort the batch.
func (s *NewReadlineShell) runBatch(ctx context.Context) error {
	scanner := bufio.NewScanner(s.IOIn)
	// Allow long queries (default token size is 64KiB which is plenty, but be safe).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		query := normalizeQuery(scanner.Text())
		if query == "" {
			continue
		}
		logger.FromContext(ctx).Debug("repl executing query", logger.String("query", query))
		if err := s.RunQuery(ctx, query, s.IOOut); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}
	}
	return scanner.Err()
}

// runInteractive drives the readline-backed prompt loop with in-memory history.
func (s *NewReadlineShell) runInteractive(ctx context.Context) error {
	rlCfg := &readline.Config{
		Prompt:          prompt,
		Stdin:           io.NopCloser(s.IOIn),
		HistoryFile:     "", // in-memory only
		InterruptPrompt: "^C",
		EOFPrompt:       "",
		AutoComplete:    s.Completion,
	}

	rl, err := readline.NewEx(rlCfg)
	if err != nil {
		return fmt.Errorf("repl: init readline: %w", err)
	}
	defer rl.Close() //nolint:errcheck

	for {
		line, readErr := rl.Readline()
		switch readErr {
		case readline.ErrInterrupt:
			// Ctrl-C at the idle prompt exits cleanly.
			return nil
		case io.EOF:
			// Ctrl-D exits cleanly.
			return nil
		case nil:
			// fallthrough to handling below
		default:
			return readErr
		}

		query := normalizeQuery(line)
		if query == "" {
			continue
		}

		if handled, exit := handleSlashCommand(query, s.IOOut); handled {
			if exit {
				return nil
			}
			continue
		}

		// Warm the column cache for this query's FROM table so subsequent
		// column completions are instant.
		if s.Completion != nil {
			s.Completion.Prefetch(query)
		}

		logger.FromContext(ctx).Debug("repl executing query", logger.String("query", query))
		if err := s.runOneInteractive(ctx, query); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}
	}
}

// runOneInteractive executes a single query with a cancellable per-query
// context so that Ctrl-C interrupts the running query and returns to the prompt
// rather than killing the whole REPL.
func (s *NewReadlineShell) runOneInteractive(ctx context.Context, query string) error {
	queryCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	done := make(chan error, 1)
	go func() {
		done <- s.RunQuery(queryCtx, query, s.IOOut)
	}()

	select {
	case err := <-done:
		return err
	case <-sigCh:
		cancel()
		<-done // wait for the query goroutine to unwind
		fmt.Fprintln(os.Stderr, "^C")
		return nil
	}
}

// handleSlashCommand processes REPL meta-commands. It returns (handled, exit):
// handled is true if the input was a meta-command (and should not be executed
// as SQL); exit is true if the REPL should terminate.
func handleSlashCommand(query string, w io.Writer) (handled, exit bool) {
	switch strings.ToLower(query) {
	case `\q`, "quit", "exit":
		return true, true
	case `\help`, "?":
		printHelp(w)
		return true, false
	default:
		return false, false
	}
}

func printHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  \\q, quit, exit   exit the REPL")
	_, _ = fmt.Fprintln(w, "  \\help, ?         show this help")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Anything else is executed as a SQL query, e.g.:")
	_, _ = fmt.Fprintln(w, "  SELECT name, namespace FROM pods WHERE status.phase != 'Running'")
}

// normalizeQuery trims surrounding whitespace and strips a single trailing
// semicolon (psql habit) so the query reaches the executor clean.
func normalizeQuery(line string) string {
	q := strings.TrimSpace(line)
	q = strings.TrimSuffix(q, ";")
	return strings.TrimSpace(q)
}
