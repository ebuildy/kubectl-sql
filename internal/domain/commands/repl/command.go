package repl

import (
	"context"
	"fmt"
	"io"
	"os"

	k8sAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/datasources/k8s"
	shellCompletionAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/shell/completion"
	shellAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/shell/readline"
	octosqlAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/sql/octosql"
	commandQuery "github.com/ebuildy/kubectl-sql/internal/domain/commands/query"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	dataSourcePort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
)

type ReplCommand struct {
	config       api.Config
	queryCommand *commandQuery.QueryCommand
	dataSource   dataSourcePort.DataSource
}

func NewReplCommand(config api.Config) (*ReplCommand, error) {
	ds, err := k8sAdapter.New(config.Kubeconfig, config.KubeContext, config.Namespace)
	if err != nil {
		return nil, fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
	}

	queryCommand, err := commandQuery.NewQueryCommandWithDataSource(config, ds)
	if err != nil {
		return nil, err
	}

	return &ReplCommand{config: config, queryCommand: queryCommand, dataSource: ds}, nil
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

	// @TODO: arch hexa should be moved to main
	shellInstance := shellAdapter.NewReadlineShell{
		RunQuery: runQueryFn,
		IOIn:     os.Stdin,
		IOOut:    os.Stdout,
		IsTTY:    interactive,
	}

	// Tab completion only matters for the interactive prompt. Build the source
	// best-effort: if the cluster is unreachable, completion is simply disabled
	// rather than aborting the REPL.
	if interactive {
		if src := shellCompletionAdapter.NewShellCompletion(ctx, r.dataSource, octosqlAdapter.FunctionNames()); src != nil {
			shellInstance.Completion = src
		}
	}

	return shellInstance.Run(ctx)
}
