// Package shell defines the port for the interactive/batch REPL shell. The
// readline adapter (internal/adapter/shell/readline) implements it; the
// composition root builds the concrete shell via the Factory and the repl
// command drives it, so the domain never imports the adapter.
package shell

import (
	"context"
	"io"

	autocomplete "github.com/ebuildy/kubectl-sql/internal/port/autocomplete"
)

// RunQueryFunc executes a single SQL query, writing rendered output to w. The
// shell calls it for each line the user enters.
type RunQueryFunc func(ctx context.Context, query string, w io.Writer) error

// Spec describes a shell session to build. The runtime values (Interactive,
// Version, ProjectURL) are known only when the REPL starts, so the Factory
// builds a Runner per session rather than at wiring time.
type Spec struct {
	RunQuery    RunQueryFunc
	In          io.Reader
	Out         io.Writer
	Interactive bool
	Version     string
	ProjectURL  string
	// Completion enables Tab autocomplete; nil disables it.
	Completion autocomplete.ShellCompletionRunner
}

// Runner runs one shell session until the user quits (or EOF in batch mode).
type Runner interface {
	Run(ctx context.Context) error
}

// Factory builds a Runner for a session Spec.
type Factory interface {
	New(spec Spec) Runner
}
