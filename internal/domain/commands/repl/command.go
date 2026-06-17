package repl

import (
	"context"
	"io"
	"os"

	commandQuery "github.com/ebuildy/kubectl-sql/internal/domain/commands/query"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	autocompletePort "github.com/ebuildy/kubectl-sql/internal/port/autocomplete"
	shellPort "github.com/ebuildy/kubectl-sql/internal/port/shell"
)

// ReplCommand drives the interactive/batch SQL shell. It depends only on ports:
// the query command (a domain use case), the completion source, and the shell
// factory. The composition root (internal/app) builds the concrete adapters and
// injects them here.
type ReplCommand struct {
	config       api.Config
	queryCommand *commandQuery.QueryCommand
	completion   autocompletePort.ShellCompletionRunner
	shells       shellPort.Factory
}

// NewReplCommand builds a ReplCommand from injected ports. config supplies the
// per-query timeout; queryCommand executes queries; completion (when non-nil)
// powers Tab autocomplete; shells builds the shell session at Run time.
func NewReplCommand(config api.Config, queryCommand *commandQuery.QueryCommand, completion autocompletePort.ShellCompletionRunner, shells shellPort.Factory) *ReplCommand {
	return &ReplCommand{config: config, queryCommand: queryCommand, completion: completion, shells: shells}
}

// Run builds a shell session and drives it. interactive selects the
// prompt-driven loop; when false the shell reads queries from stdin in batch
// mode. Completion is attached only in interactive mode.
func (r *ReplCommand) Run(ctx context.Context, interactive bool, version, projectURL string) error {
	runQueryFn := func(ctx context.Context, query string, w io.Writer) error {
		queryCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
		defer cancel()
		return r.queryCommand.RunWithWriter(queryCtx, query, w)
	}

	spec := shellPort.Spec{
		RunQuery:    runQueryFn,
		In:          os.Stdin,
		Out:         os.Stdout,
		Interactive: interactive,
		Version:     version,
		ProjectURL:  projectURL,
	}

	// Tab completion only matters for the interactive prompt.
	if interactive {
		spec.Completion = r.completion
	}

	return r.shells.New(spec).Run(ctx)
}
