// Package sql defines the SQL-engine port: a library-free interface for running
// a SQL query and rendering its result. Consumers depend only on this package;
// the octosql binding lives behind the adapter in internal/adapter/sql/octosql,
// so the engine can be swapped without touching call sites.
package sql

import (
	"context"
	"io"
)

// Query is a library-free description of a query to run.
type Query struct {
	SQL       string
	Output    string // "table" | "json" | "csv"
	Namespace string
	PageSize  int
	NoColor   bool
}

// Engine runs SQL queries against a data source and renders the result.
type Engine interface {
	// Execute runs the query and writes the rendered result to w.
	Execute(ctx context.Context, q Query, w io.Writer) error
}
