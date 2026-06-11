// Package logger is the logging port: a domain-owned Logger interface plus a
// library-agnostic Field type and constructors. Application code depends only
// on this package. The concrete logging library lives behind an adapter (see
// internal/adapter/logger/zap) so it can be swapped without touching call sites.
package logger

import "time"

// Field is a structured log field, decoupled from any logging library.
type Field struct {
	Key   string
	Value any
}

// String builds a string-valued field.
func String(key, value string) Field { return Field{Key: key, Value: value} }

// Int builds an int-valued field.
func Int(key string, value int) Field { return Field{Key: key, Value: value} }

// Err builds a field carrying an error under the conventional "error" key.
func Err(err error) Field { return Field{Key: "error", Value: err} }

// Any builds a field for an arbitrary value (fallback).
func Any(key string, value any) Field { return Field{Key: key, Value: value} }

// Duration builds a field recording an elapsed duration in milliseconds under
// the key "<key>_ms".
func Duration(key string, d time.Duration) Field {
	return Field{Key: key + "_ms", Value: d.Milliseconds()}
}

// Logger is the logging port. All application code calls these methods only.
type Logger interface {
	// Trace logs very high-volume diagnostic detail (e.g. raw payloads),
	// below Debug. Only emitted at the highest verbosity.
	Trace(msg string, fields ...Field)
	// TraceEnabled reports whether Trace-level logging is active, so callers
	// can skip building expensive fields (e.g. JSON-marshaling) when it isn't.
	TraceEnabled() bool
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	// With returns a child logger that includes the given fields on every entry.
	With(fields ...Field) Logger
	// Sync flushes any buffered entries. No-op for loggers that do not buffer.
	Sync() error
}

// Options configures logger construction. Verbosity maps to a level:
// 0 -> error, 1 -> info, 2 -> debug, >=3 -> trace. NoColor disables ANSI level coloring.
type Options struct {
	Verbosity int
	NoColor   bool
}

// nopLogger is a Logger that discards everything.
type nopLogger struct{}

func (nopLogger) Trace(string, ...Field) {}
func (nopLogger) TraceEnabled() bool     { return false }
func (nopLogger) Debug(string, ...Field) {}
func (nopLogger) Info(string, ...Field)  {}
func (nopLogger) Error(string, ...Field) {}
func (n nopLogger) With(...Field) Logger { return n }
func (nopLogger) Sync() error            { return nil }

// Nop returns a no-op Logger, used for tests and contexts without a logger.
func Nop() Logger { return nopLogger{} }
