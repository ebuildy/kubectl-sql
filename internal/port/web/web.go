// Package web defines the driving ports the web UI adapter depends on. The
// HTTP adapter (internal/adapter/web) accepts requests and drives these ports;
// the domain command (internal/domain/commands/ui) implements them over the
// existing SQL engine and completion source. Keeping the ports here lets the
// HTTP adapter be tested with fakes and free of k8s/octosql imports.
package web

import (
	"context"
	"net"
)

// QueryResult is the shape returned to the browser: an ordered list of column
// names plus the result rows as decoded JSON objects (mirroring --output json).
type QueryResult struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

// QueryRunner runs a SQL query in JSON mode and re-shapes the rendered output
// into a QueryResult. It returns an *Error when the underlying engine offers a
// typo-correction suggestion so handlers can serialize it.
type QueryRunner interface {
	RunJSON(ctx context.Context, sql string) (QueryResult, error)
}

// Completer returns completion candidates for the editor line and cursor
// position, as full tokens ready to insert.
type Completer interface {
	Complete(line string, pos int) []string
}

// Error is a structured query error carrying a user-facing message and, when
// the engine produced a single-token typo correction, the suggestion text and
// corrected SQL so the UI can offer it.
type Error struct {
	Message      string
	Suggestion   string
	CorrectedSQL string
}

func (e *Error) Error() string { return e.Message }

// Server is the driving port for the HTTP server lifecycle. The web adapter
// (internal/adapter/web) implements it; the composition root builds the
// concrete server and injects it into the ui command so the domain never
// imports the adapter.
type Server interface {
	// Listen binds the configured address and returns the listener.
	Listen() (net.Listener, error)
	// Serve serves HTTP on ln until Shutdown, returning http.ErrServerClosed on
	// a clean shutdown.
	Serve(ln net.Listener) error
	// Shutdown gracefully stops the server.
	Shutdown(ctx context.Context) error
}
