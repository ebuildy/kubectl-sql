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

// mapFakeDS serves a pod whose metadata.labels is a FieldTypeMap with two keys.
type mapFakeDS struct{}

func (mapFakeDS) Resolve(_ context.Context, _ string) (k8sport.Resource, error) {
	return k8sport.Resource{Name: "pods", Version: "v1", Namespaced: true}, nil
}
func (mapFakeDS) Resources(context.Context) ([]k8sport.Resource, error) { return nil, nil }
func (mapFakeDS) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
	return []schema.Field{
		{Name: "metadata", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			// A map declares no fixed key contract — keys vary per row.
			{Name: "labels", Type: schema.FieldTypeMap},
		}},
	}, nil
}
func (mapFakeDS) Delete(_ context.Context, _ k8sport.Resource, _, _ string, _ k8sport.DeleteOptions) error {
	return nil
}

func (mapFakeDS) List(_ context.Context, _ k8sport.Resource, _ k8sport.ListOptions, fn func([]map[string]any) error) error {
	return fn([]map[string]any{
		{
			"metadata": map[string]any{
				"name":   "nginx",
				"labels": map[string]any{"app": "nginx", "tier": "web"},
			},
		},
		{
			"metadata": map[string]any{
				"name":   "redis",
				"labels": map[string]any{"tier": "db", "env": "prod", "vendor": "valkey"},
			},
		},
	})
}

// TestMapField proves a map field can be accessed via -> and returned as a JSON object.
func TestMapField(t *testing.T) {
	eng := New(portsql.Config{Output: "json"}, mapFakeDS{}, nil)
	var buf strings.Builder

	err := eng.Execute(context.Background(),
		portsql.Query{SQL: "SELECT metadata->name AS name, metadata->labels AS labels FROM pods"},
		&buf)
	require.NoError(t, err, "execute: %s", buf.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "JSON: %s", buf.String())
	require.Len(t, rows, 2)

	assert.Equal(t, "nginx", rows[0]["name"], "map key access via ->")
	assert.Equal(t, map[string]any{"app": "nginx", "tier": "web"}, rows[0]["labels"], "map field should be returned as a JSON object")

	assert.Equal(t, "redis", rows[1]["name"], "map key access via ->")
	assert.Equal(t, map[string]any{"tier": "db", "env": "prod", "vendor": "valkey"}, rows[1]["labels"], "map field should be returned as a JSON object")
}
