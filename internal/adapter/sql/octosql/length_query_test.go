package octosql

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// lengthFakeDS serves a single pod whose metadata.labels is a struct (one key)
// and spec.volumes is a list (one element). It drives the full query pipeline so
// length() is exercised end-to-end, not just at the descriptor level.
type lengthFakeDS struct{}

func (lengthFakeDS) Resolve(_ context.Context, _ string) (k8sport.Resource, error) {
	return k8sport.Resource{Name: "pods", Version: "v1", Namespaced: true}, nil
}
func (lengthFakeDS) Resources(context.Context) ([]k8sport.Resource, error) { return nil, nil }
func (lengthFakeDS) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
	return []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "metadata", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "labels", Type: schema.FieldTypeObject, SubFields: []schema.Field{
				{Name: "app", Type: schema.FieldTypeString},
			}},
		}},
		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "volumes", Type: schema.FieldTypeList},
		}},
	}, nil
}
func (lengthFakeDS) Delete(_ context.Context, _ k8sport.Resource, _, _ string, _ k8sport.DeleteOptions) error {
	return nil
}

func (lengthFakeDS) List(_ context.Context, _ k8sport.Resource, _ k8sport.ListOptions, fn func([]map[string]any) error) error {
	return fn([]map[string]any{
		{
			"metadata": map[string]any{
				"name":   "nginx",
				"labels": map[string]any{"app": "nginx"},
			},
			"spec": map[string]any{
				"volumes": []any{
					map[string]any{"name": "config", "configMap": map[string]any{"name": "nginx-config"}},
				},
			},
		},
	})
}

// TestLength_CountsNestedStructAndList drives the full octosql pipeline and
// asserts length() returns the number of labels (struct fields) and the number
// of volumes (list elements), and that the list column renders as a JSON array.
func TestLength_CountsNestedStructAndList(t *testing.T) {
	eng := New(portsql.Config{Output: "json"}, lengthFakeDS{})
	var buf strings.Builder
	err := eng.Execute(context.Background(),
		portsql.Query{SQL: "SELECT length(metadata->labels) AS nlabels, length(spec->volumes) AS nvolumes, spec->volumes AS volumes FROM pods"},
		&buf)
	require.NoError(t, err, "execute")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "output is JSON: %s", buf.String())
	require.Len(t, rows, 1)

	assert.EqualValues(t, 1, rows[0]["nlabels"], "length(metadata->labels) counts labels")
	assert.EqualValues(t, 1, rows[0]["nvolumes"], "length(spec->volumes) counts volumes")

	vols, ok := rows[0]["volumes"].([]any)
	require.True(t, ok, "spec->volumes renders as a JSON array, got %T", rows[0]["volumes"])
	assert.Len(t, vols, 1, "one volume element")
}
