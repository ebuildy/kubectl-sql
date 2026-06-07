package repl

import (
	"context"
	"io"
	"os"

	"github.com/ebuildy/kubectl-sql/internal/domain/autocomplete"
	commandQuery "github.com/ebuildy/kubectl-sql/internal/domain/commands/query"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	"github.com/ebuildy/kubectl-sql/internal/repl"
)

type ReplCommand struct {
	config       api.Config
	queryCommand *commandQuery.QueryCommand
}

func NewReplCommand(config api.Config) (*ReplCommand, error) {
	queryCommand, err := commandQuery.NewQueryCommand(config)
	if err != nil {
		return nil, err
	}

	return &ReplCommand{config: config, queryCommand: queryCommand}, nil
}

// runREPL wires the REPL package to the cobra command, forwarding all flags via
// a closure over runQueryWithWriter. interactive selects the prompt-driven loop;
// when false the REPL reads queries from stdin in batch mode.
func (r *ReplCommand) Run(ctx context.Context, interactive bool) error {

	runQueryFn := func(ctx context.Context, query string, w io.Writer) error {
		timeout := r.config.Timeout
		queryCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return r.queryCommand.RunWithWriter(queryCtx, query, w)
	}

	replCfg := repl.Config{
		RunQuery: runQueryFn,
		Stdin:    os.Stdin,
		IsTTY:    interactive,
	}

	// Tab completion only matters for the interactive prompt. Build the source
	// best-effort: if the cluster is unreachable, completion is simply disabled
	// rather than aborting the REPL.
	if interactive {
		if src := autocomplete.NewCompletionSource(ctx, r.config); src != nil {
			replCfg.Completion = src
		}
	}

	return repl.Run(ctx, replCfg, os.Stdout)
}
