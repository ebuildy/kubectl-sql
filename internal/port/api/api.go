package api

import (
	"context"
	"io"
	"time"
)

// Package api defines the public API of the application, which is consumed by cmd/ and tested by internal/adapter/.
type Config struct {
	Kubeconfig  string
	KubeContext string
	Namespace   string
	Output      string
	PageSize    int
	NoColor     bool
	Timeout     time.Duration
	Watch       bool
	Out         io.Writer
}

// Logger is the logging port. All application code calls these methods only.
type Command interface {
	Run(ctx context.Context, cfg Config, query string, w io.Writer) error
}
