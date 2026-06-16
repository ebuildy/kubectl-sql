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
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chzyer/readline"

	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"

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
	// Version is the build version string reported by the /version command
	// (defaults to "dev" when not injected at build time).
	Version string
	// ProjectURL is the project home reported by the /version command.
	ProjectURL string
	// clearHistory resets the interactive recall history (wired to
	// rl.ResetHistory in runInteractive). Nil off a TTY, making /history-clear a
	// no-op there.
	clearHistory func()
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
		// Control commands are honored off a TTY too: quit/exit and /quit stop
		// the batch, /clear is a no-op, and the rest behave as on a TTY.
		if handled, exit := s.handleCommand(ctx, query, s.IOOut); handled {
			if exit {
				return nil
			}
			continue
		}
		logger.FromContext(ctx).Debug("repl executing query", logger.String("query", query))
		if err := s.RunQuery(ctx, query, s.IOOut); err != nil {
			// Batch mode cannot pre-fill an editable prompt, so print the full
			// suggestion (including the corrected query) without running it.
			var se *sqlPort.SuggestionError
			if errors.As(err, &se) {
				fmt.Fprintln(os.Stderr, se.Suggestion.Message())
				continue
			}
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

	// Wire /history-clear to the readline instance's history reset.
	s.clearHistory = rl.ResetHistory

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

		if handled, exit := s.handleCommand(ctx, query, s.IOOut); handled {
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
		err := s.runOneInteractive(ctx, query)
		// A typo with a close valid match: instead of prompting, show the
		// diagnostic and pre-fill the corrected query into the next prompt so the
		// user can press Enter to run it or edit it first.
		var se *sqlPort.SuggestionError
		if errors.As(err, &se) {
			handleInteractiveSuggestion(se, rl, os.Stderr)
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}
	}
}

// stdinFiller is the subset of *readline.Instance used to pre-fill the next
// prompt; abstracted so the suggestion handling can be unit-tested.
type stdinFiller interface {
	WriteStdin([]byte) (int, error)
}

// handleInteractiveSuggestion prints the suggestion diagnostic and pre-fills the
// corrected query into the next prompt for editing. The corrected query is not
// run until the user confirms it by pressing Enter.
func handleInteractiveSuggestion(se *sqlPort.SuggestionError, rl stdinFiller, errOut io.Writer) {
	_, _ = fmt.Fprintln(errOut, se.Suggestion.Hint())
	_, _ = rl.WriteStdin([]byte(se.Suggestion.CorrectedSQL))
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

// clearScreen is the ANSI sequence that moves the cursor home and clears the
// screen (equivalent to Ctrl-L). Written only when stdout is a TTY.
const clearScreen = "\033[H\033[2J"

// handleCommand processes REPL control commands. It returns (handled, exit):
// handled is true if the input was a command (and must not be executed as SQL);
// exit is true if the REPL should terminate.
//
// Dispatch rules:
//   - bare-word "quit"/"exit" (case-insensitive) exit, as convenience aliases;
//   - any input whose first character is '/' is a slash command (recognized ones
//     act; unrecognized ones print guidance and return to the prompt);
//   - everything else is not a command and is left for the SQL engine.
//
// The input is expected to be already trimmed (see normalizeQuery), so a leading
// '/' is the first non-space character of the original line.
func (s *NewReadlineShell) handleCommand(ctx context.Context, input string, w io.Writer) (handled, exit bool) {
	switch strings.ToLower(input) {
	case "quit", "exit":
		return true, true
	}

	if !strings.HasPrefix(input, "/") {
		return false, false
	}

	switch strings.ToLower(input) {
	case "/quit":
		return true, true
	case "/help":
		printHelp(w)
	case "/version":
		_, _ = fmt.Fprintf(w, "%s\n%s\n", s.versionString(), s.projectURLString())
	case "/tables":
		if err := s.RunQuery(ctx, "SHOW TABLES", w); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}
	case "/clear":
		// Clear the screen only on a TTY; off a TTY emit nothing so piped
		// output is not polluted with escape codes. History is untouched.
		if s.IsTTY {
			_, _ = io.WriteString(w, clearScreen)
		}
	case "/history-clear":
		// Reset the recall history; nil off a TTY where no history exists.
		if s.clearHistory != nil {
			s.clearHistory()
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %s, try /help\n", input)
	}
	return true, false
}

// versionString returns the build version, defaulting to "dev" when not injected.
func (s *NewReadlineShell) versionString() string {
	if s.Version == "" {
		return "dev"
	}
	return s.Version
}

// projectURLString returns the configured project URL, defaulting to the
// canonical home when not set.
func (s *NewReadlineShell) projectURLString() string {
	if s.ProjectURL == "" {
		return "https://github.com/ebuildy/kubectl-sql"
	}
	return s.ProjectURL
}

func printHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  /quit          exit the REPL (quit, exit also work)")
	_, _ = fmt.Fprintln(w, "  /clear         clear the screen")
	_, _ = fmt.Fprintln(w, "  /history-clear clear the recall history")
	_, _ = fmt.Fprintln(w, "  /help          show this help")
	_, _ = fmt.Fprintln(w, "  /version       show version")
	_, _ = fmt.Fprintln(w, "  /tables        list tables")
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
