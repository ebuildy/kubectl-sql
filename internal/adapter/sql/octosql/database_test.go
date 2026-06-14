package octosql

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// fakeDataSource implements the k8s DataSource port with a fixed resolution table.
type fakeDataSource struct {
	resolvable map[string]bool
}

func (f *fakeDataSource) Resolve(_ context.Context, table string) (k8sport.Resource, error) {
	if f.resolvable[table] {
		return k8sport.Resource{Name: "pods", Version: "v1", Namespaced: true}, nil
	}
	return k8sport.Resource{}, fmt.Errorf("unknown resource %q", table)
}

func (f *fakeDataSource) Resources(context.Context) ([]k8sport.Resource, error) { return nil, nil }

func (f *fakeDataSource) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
	return []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "namespace", Type: schema.FieldTypeString},
	}, nil
}

func (f *fakeDataSource) List(_ context.Context, _ k8sport.Resource, _ k8sport.ListOptions, _ func([]map[string]any) error) error {
	return nil
}

func (f *fakeDataSource) Delete(_ context.Context, _ k8sport.Resource, _, _ string, _ k8sport.DeleteOptions) error {
	return nil
}

func newFakeDataSource() *fakeDataSource {
	return &fakeDataSource{resolvable: map[string]bool{"pods": true, "pod": true, "deployments": true}}
}

func TestGetTable_PluralResolves(t *testing.T) {
	db := NewKubernetesDatabase(newFakeDataSource(), "", 500)
	impl, sch, err := db.GetTable(context.Background(), "pods", nil)
	require.NoError(t, err)
	assert.NotNil(t, impl)
	assert.NotEmpty(t, sch.Fields)
}

func TestGetTable_SingularResolves(t *testing.T) {
	db := NewKubernetesDatabase(newFakeDataSource(), "", 500)
	impl, _, err := db.GetTable(context.Background(), "pod", nil)
	require.NoError(t, err)
	assert.NotNil(t, impl)
}

func TestGetTable_UnknownReturnsError(t *testing.T) {
	db := NewKubernetesDatabase(newFakeDataSource(), "", 500)
	_, _, err := db.GetTable(context.Background(), "doesnotexist", nil)
	require.Error(t, err)
}
