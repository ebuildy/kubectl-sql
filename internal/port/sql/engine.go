// Package sql defines the SQL-engine port: a library-free interface for running
// a SQL query and rendering its result. Consumers depend only on this package;
// the octosql binding lives behind the adapter in internal/adapter/sql/octosql,
// so the engine can be swapped without touching call sites.
package sql

import (
	"context"
	"fmt"
	"io"
)

// Query is a library-free description of a query to run.
type Query struct {
	SQL string
}

// SuggestionKind identifies which stage produced a typo-correction suggestion.
type SuggestionKind string

const (
	SuggestionKindKeyword SuggestionKind = "keyword"
	SuggestionKindTable   SuggestionKind = "table"
	SuggestionKindField   SuggestionKind = "field"
	// SuggestionKindSyntax is a structural fix (not a token similarity match),
	// e.g. closing an unterminated string literal.
	SuggestionKindSyntax SuggestionKind = "syntax"
	// SuggestionKindDotNotation flags dotted sub-field access (spec.annotations)
	// that should use the arrow operator (spec->annotations).
	SuggestionKindDotNotation SuggestionKind = "dot-notation"
)

// Suggestion is a single-token typo correction the engine offers when a query
// fails because of one mistyped keyword, table, or field for which a close valid
// alternative exists. It is library-free so callers (one-shot CLI, REPL) can act
// on it uniformly.
type Suggestion struct {
	Kind         SuggestionKind
	Typo         string // the mistyped token as written
	Suggestion   string // the closest valid token
	CorrectedSQL string // the original query with the typo substituted
}

// Message returns the user-facing one-line suggestion, ending in the corrected
// query, e.g. "error: field staus does not exist, run this query instead ? SELECT status FROM pods".
// It is used where the corrected query cannot be offered for inline editing
// (the one-shot CLI prompt and batch mode).
func (s Suggestion) Message() string {
	switch s.Kind {
	case SuggestionKindKeyword:
		return fmt.Sprintf("error: did you mean %s? run this query instead ? %s", s.Suggestion, s.CorrectedSQL)
	case SuggestionKindTable:
		return fmt.Sprintf("error: table %s does not exist, run this query instead ? %s", s.Typo, s.CorrectedSQL)
	case SuggestionKindSyntax:
		return fmt.Sprintf("error: missing closing quote, run this query instead ? %s", s.CorrectedSQL)
	case SuggestionKindDotNotation:
		return fmt.Sprintf("error: use -> not . to access object sub-fields, run this query instead ? %s", s.CorrectedSQL)
	default:
		return fmt.Sprintf("error: field %s does not exist, run this query instead ? %s", s.Typo, s.CorrectedSQL)
	}
}

// Hint returns the diagnostic part of the suggestion WITHOUT the corrected
// query, e.g. "error: field staus does not exist, did you mean status?". The
// interactive REPL prints this and pre-fills the corrected query into the input
// line for editing, so repeating the query in the message would be redundant.
func (s Suggestion) Hint() string {
	switch s.Kind {
	case SuggestionKindKeyword:
		return fmt.Sprintf("error: did you mean %s?", s.Suggestion)
	case SuggestionKindTable:
		return fmt.Sprintf("error: table %s does not exist, did you mean %s?", s.Typo, s.Suggestion)
	case SuggestionKindSyntax:
		return "error: missing closing quote"
	case SuggestionKindDotNotation:
		return "error: use -> not . to access object sub-fields"
	default:
		return fmt.Sprintf("error: field %s does not exist, did you mean %s?", s.Typo, s.Suggestion)
	}
}

// SuggestionError wraps the original failure together with a typo-correction
// Suggestion. Execute returns it (instead of the bare error) when a single
// mistyped token has a close valid match. Callers type-assert via errors.As to
// present the suggestion and, on confirmation, run the corrected query. It
// unwraps to the original error so existing error handling still applies.
type SuggestionError struct {
	Suggestion Suggestion
	Err        error
}

func (e *SuggestionError) Error() string { return e.Err.Error() }
func (e *SuggestionError) Unwrap() error { return e.Err }

// Engine runs SQL queries against a data source and renders the result.
type Engine interface {
	// Execute runs the query and writes the rendered result to w.
	Execute(ctx context.Context, q Query, w io.Writer) error
}

// EngineFactory builds an Engine configured for a given Config. The SQL engine
// is created per call because the output mode varies (the user's --output, the
// web UI's "json", the mutator's "csv"); a factory lets a consumer keep that
// Config policy while obtaining an Engine without importing any engine library.
// The concrete factory lives behind the adapter (internal/adapter/sql/octosql).
type EngineFactory interface {
	New(cfg Config) Engine
}

// Config holds configuration options for the SQL engine.
type Config struct {
	Output        string // "table" | "json" | "csv"
	Namespace     string
	PageSize      int
	NoColor       bool
	DisableBeauty bool // render struct cells as compact uncolored JSON
}

// ColorEnabled reports whether ANSI coloring should be applied, given whether
// the final output destination is a terminal.
func (c Config) ColorEnabled(isTTY bool) bool {
	return isTTY && !c.NoColor && !c.DisableBeauty
}
