package autocomplete

import (
	"context"
	"strings"

	k8sadapter "github.com/ebuildy/kubectl-sql/internal/adapter/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/repl"
)

// cliCompletionSource implements repl.CompletionSource over the k8s DataSource port.
type cliCompletionSource struct {
	ctx context.Context
	ds  k8sport.DataSource
}

// NewCompletionSource builds a completion source, returning nil if the cluster
// connection fails (completion is then disabled).
func NewCompletionSource(ctx context.Context, config api.Config) repl.CompletionSource {
	ds, err := k8sadapter.New(config.Kubeconfig, config.KubeContext, config.Namespace)
	if err != nil {
		return nil
	}
	return &cliCompletionSource{ctx: ctx, ds: ds}
}

// Tables returns queryable resource names via the port.
func (s *cliCompletionSource) Tables() []string {
	resources, err := s.ds.Resources(s.ctx)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(resources))
	for _, r := range resources {
		names = append(names, r.Name)
	}
	return names
}

// Columns returns the column names for a table, or nil if it cannot be resolved.
func (s *cliCompletionSource) Columns(table string) []string {
	resource, err := s.ds.Resolve(s.ctx, strings.ToLower(table))
	if err != nil {
		return nil
	}
	fields, err := s.ds.InferSchema(s.ctx, resource)
	if err != nil {
		return nil
	}
	cols := make([]string, 0, len(fields))
	for _, f := range fields {
		cols = append(cols, f.Name)
	}
	return cols
}
