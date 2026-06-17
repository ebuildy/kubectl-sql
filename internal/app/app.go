// Package app is the composition root. It owns every concrete adapter
// construction (the k8s DataSource, the SQL engine factory, the mutator, the
// spellchecker, the completion source, the readline shell, and the web server)
// and injects them, as ports, into the domain command constructors. The domain
// (internal/domain/...) depends only on ports and never imports any adapter;
// cmd depends on this package for wiring.
package app

import (
	"context"
	"fmt"

	k8sAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/datasources/k8s"
	shellCompletionAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/shell/completion"
	shellAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/shell/readline"
	spellcheckerAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/spellchecker"
	mutatorAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/sql/mutator"
	octosqlAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/sql/octosql"
	webAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/web"
	commandQuery "github.com/ebuildy/kubectl-sql/internal/domain/commands/query"
	commandRepl "github.com/ebuildy/kubectl-sql/internal/domain/commands/repl"
	commandUI "github.com/ebuildy/kubectl-sql/internal/domain/commands/ui"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	k8sPort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// NewQueryCommand wires the one-shot query command: a k8s DataSource, an octosql
// engine factory (with spellchecker for typo suggestions), and the csv-config
// mutator for DELETE.
func NewQueryCommand(ctx context.Context, cfg api.Config) (*commandQuery.QueryCommand, error) {
	ds, err := newDataSource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
	}

	engines := octosqlAdapter.NewFactory(ds, spellcheckerAdapter.New())
	mut := newMutator(ds, cfg)

	return commandQuery.NewQueryCommand(cfg, ds, engines, mut, false), nil
}

// NewReplCommand wires the REPL: the query command (in REPL mode), a best-effort
// completion source, and the readline shell factory.
func NewReplCommand(ctx context.Context, cfg api.Config) (*commandRepl.ReplCommand, error) {
	ds, err := newDataSource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
	}

	engines := octosqlAdapter.NewFactory(ds, spellcheckerAdapter.New())
	mut := newMutator(ds, cfg)
	queryCommand := commandQuery.NewQueryCommand(cfg, ds, engines, mut, true)

	completion := shellCompletionAdapter.NewShellCompletion(ctx, ds, octosqlAdapter.FunctionNames())
	shells := shellAdapter.NewFactory()

	return commandRepl.NewReplCommand(cfg, queryCommand, completion, shells), nil
}

// NewUICommand wires the web UI: a k8s DataSource, an octosql engine factory, a
// completion source, and the HTTP server built around the command. A
// cluster-connection failure is returned as a non-zero ExitError so no server is
// started.
func NewUICommand(ctx context.Context, cfg api.Config, addr string) (*commandUI.UICommand, error) {
	ds, err := newDataSource(ctx, cfg)
	if err != nil {
		return nil, api.ExitError{Code: 2, Err: fmt.Errorf("kubectl-sql: connect to cluster: %w", err)}
	}

	engines := octosqlAdapter.NewFactory(ds, spellcheckerAdapter.New())
	completion := shellCompletionAdapter.NewShellCompletion(ctx, ds, octosqlAdapter.FunctionNames())

	ui := commandUI.NewUICommand(cfg, ds, engines, completion, addr)
	srv := webAdapter.NewServer(ui, ui, addr, commandQuery.IsDeleteStatement)
	ui.SetServer(srv)

	return ui, nil
}

// newDataSource connects to the cluster, mapping the CLI config onto the k8s
// adapter constructor. Callers wrap the error to suit their exit semantics.
func newDataSource(ctx context.Context, cfg api.Config) (k8sPort.DataSource, error) {
	return k8sAdapter.New(ctx, cfg.Kubeconfig, cfg.KubeContext, cfg.Namespace)
}

// newMutator builds the DELETE mutator: a CSV-rendering SELECT engine (so the
// resolved deletion set parses robustly regardless of the user's --output) over
// the given DataSource. The namespace/page-size come from the CLI config.
func newMutator(ds k8sPort.DataSource, cfg api.Config) sqlPort.Mutator {
	csvCfg := sqlPort.Config{
		Output:    "csv",
		Namespace: cfg.Namespace,
		PageSize:  cfg.PageSize,
		NoColor:   true,
	}
	csvEngine := octosqlAdapter.NewFactory(ds, nil).New(csvCfg)
	return mutatorAdapter.New(csvEngine, ds)
}
