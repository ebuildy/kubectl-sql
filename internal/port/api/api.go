package api

import (
	"context"
	"io"
	"time"
)

// Package api defines the public API of the application, which is consumed by cmd/ and tested by internal/adapter/.
type Config struct {
	Kubeconfig    string
	KubeContext   string
	Namespace     string
	Output        string
	PageSize      int
	NoColor       bool
	DisableBeauty bool
	Timeout       time.Duration
	Watch         bool
	DryRun        bool
	Yes           bool
	Out           io.Writer
}

// Logger is the logging port. All application code calls these methods only.
type Command interface {
	Run(ctx context.Context, cfg Config, query string, w io.Writer) error
}

// ExitError carries a specific process exit code alongside an error, so a
// command can signal e.g. a Kubernetes API failure (exit 2) distinctly from a
// generic query/parse error (exit 1). cmd.Execute unwraps it via errors.As.
type ExitError struct {
	Code int
	Err  error
}

func (e ExitError) Error() string { return e.Err.Error() }
func (e ExitError) Unwrap() error { return e.Err }
