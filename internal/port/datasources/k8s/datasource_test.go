package k8s

import (
	"context"
	"testing"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// fakeDataSource verifies the port can be satisfied with no k8s.io dependency.
type fakeDataSource struct{}

func (fakeDataSource) Resolve(context.Context, string) (Resource, error) { return Resource{}, nil }
func (fakeDataSource) Resources(context.Context) ([]Resource, error)     { return nil, nil }
func (fakeDataSource) InferSchema(context.Context, Resource) ([]schema.Field, error) {
	return nil, nil
}
func (fakeDataSource) List(context.Context, Resource, ListOptions, func([]map[string]any) error) error {
	return nil
}

func TestPortIsSatisfiable(t *testing.T) {
	var _ DataSource = fakeDataSource{}
}
