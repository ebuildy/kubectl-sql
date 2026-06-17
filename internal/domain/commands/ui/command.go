// Package ui wires the existing data source, SQL engine, and completion source
// into the web HTTP adapter and owns the server lifecycle. It mirrors how
// ReplCommand wires the readline adapter: the HTTP adapter is a primary/driving
// adapter, and this command implements the driving ports it depends on
// (internal/port/web) by delegating to the same adapters the CLI uses.
package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/ebuildy/kubectl-sql/internal/port/api"
	autocompletePort "github.com/ebuildy/kubectl-sql/internal/port/autocomplete"
	k8sPort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
	webPort "github.com/ebuildy/kubectl-sql/internal/port/web"
)

// UICommand starts a local web server backed by the same cluster wiring as the
// CLI. It implements the web driving ports (QueryRunner, Completer). All
// adapters are injected by the composition root (internal/app); the server is
// set via SetServer once it has been built around this command.
type UICommand struct {
	config     api.Config
	dataSource k8sPort.DataSource
	engines    sqlPort.EngineFactory
	completion autocompletePort.ShellCompletionRunner
	addr       string
	server     webPort.Server
}

// NewUICommand builds a UICommand from injected ports. The composition root
// wires the DataSource, EngineFactory, and completion source, then constructs
// the web server around this command and injects it via SetServer.
func NewUICommand(config api.Config, ds k8sPort.DataSource, engines sqlPort.EngineFactory, completion autocompletePort.ShellCompletionRunner, addr string) *UICommand {
	return &UICommand{config: config, dataSource: ds, engines: engines, completion: completion, addr: addr}
}

// SetServer injects the web server. The composition root calls it after building
// the server with this command as the query/completion backend (a two-phase
// construction, since the server needs this command and vice versa).
func (c *UICommand) SetServer(s webPort.Server) { c.server = s }

// RunJSON implements webPort.QueryRunner. It runs the query through the octosql
// engine in JSON output mode and re-shapes the rendered output into a
// QueryResult. A single-token typo surfaces as a webPort.Error carrying the
// suggestion and corrected SQL.
func (c *UICommand) RunJSON(ctx context.Context, sql string) (webPort.QueryResult, error) {
	cfg := sqlPort.Config{
		Output:    "json",
		Namespace: c.config.Namespace,
		PageSize:  c.config.PageSize,
		NoColor:   true,
	}
	eng := c.engines.New(cfg)

	var buf bytes.Buffer
	if err := eng.Execute(ctx, sqlPort.Query{SQL: sql}, &buf); err != nil {
		var se *sqlPort.SuggestionError
		if errors.As(err, &se) {
			return webPort.QueryResult{}, &webPort.Error{
				Message:      se.Suggestion.Hint(),
				Suggestion:   se.Suggestion.Hint(),
				CorrectedSQL: se.Suggestion.CorrectedSQL,
			}
		}
		return webPort.QueryResult{}, err
	}

	rows := []map[string]any{}
	// An empty result set renders nothing (no JSON array); treat that as zero rows.
	if buf.Len() > 0 {
		if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
			return webPort.QueryResult{}, fmt.Errorf("kubectl-sql: decode query result: %w", err)
		}
	}

	return webPort.QueryResult{Columns: columnsOf(rows), Rows: rows}, nil
}

// columnsOf derives the ordered column list from the union of row keys. The
// JSON renderer emits rows as objects (key order not preserved), so columns are
// sorted for a deterministic, stable header — matching what --output json shows.
func columnsOf(rows []map[string]any) []string {
	seen := map[string]struct{}{}
	for _, row := range rows {
		for k := range row {
			seen[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// Complete implements webPort.Completer. It wraps ShellCompletionRunner.Do,
// turning its readline-style suffix candidates into full tokens by re-prefixing
// the partial word under the cursor. Prefetch is called best-effort to warm the
// column cache for the query's FROM table.
func (c *UICommand) Complete(line string, pos int) []string {
	if c.completion == nil {
		return nil
	}
	c.completion.Prefetch(line)

	runes := []rune(line)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	suffixes, length := c.completion.Do(runes, pos)
	if length < 0 || length > pos {
		length = 0
	}
	prefix := string(runes[pos-length : pos])

	out := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		out = append(out, prefix+string(s))
	}
	return out
}

// Run starts the web server, prints the listen URL to stderr, opens the page in
// the default browser, and blocks until an interrupt signal (SIGINT/SIGTERM)
// triggers a graceful shutdown. When initialQuery is non-empty it is passed to
// the page via the ?sql= query string so the editor opens pre-filled. A bind
// failure is returned as a non-zero ExitError.
func (c *UICommand) Run(ctx context.Context, initialQuery string) error {
	srv := c.server

	ln, err := srv.Listen()
	if err != nil {
		return api.ExitError{Code: 2, Err: fmt.Errorf("kubectl-sql: bind %s: %w", c.addr, err)}
	}

	fmt.Fprintf(os.Stderr, "kubectl-sql UI listening on http://%s\n", ln.Addr())

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	// Best-effort: open the page in the default browser. The listener is already
	// bound, so queued connections will be served once Serve runs. A failure is
	// non-fatal — the URL was already printed for manual navigation.
	pageURL := browserURL(ln.Addr(), initialQuery)
	if err := openBrowser(pageURL); err != nil {
		fmt.Fprintf(os.Stderr, "could not open browser automatically: %v (open %s manually)\n", err, pageURL)
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return api.ExitError{Code: 2, Err: fmt.Errorf("kubectl-sql: web server: %w", err)}
	}
}
