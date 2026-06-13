package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// TestStrategicSchemaProvider_SwaggerLayerEnrichesPods exercises the
// strategicSchemaProvider with nil discovery/dynamic clients, so only the
// default baseline and the embedded swagger snapshot (Layer 1) contribute
// fields. For a standard resource (pods), spec/status must come back with
// full nested structure from the embedded snapshot alone.
func TestStrategicSchemaProvider_SwaggerLayerEnrichesPods(t *testing.T) {
	provider := newStrategicSchemaProvider(context.Background(), "", nil, nil)

	fields, err := provider.Provide(context.Background(), k8sschema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"})
	require.NoError(t, err)

	spec := findField(t, fields, "spec")
	assert.NotEmpty(t, spec.SubFields, "spec must be enriched with nested fields from the embedded swagger snapshot")
	containers := findField(t, spec.SubFields, "containers")
	assert.Equal(t, schema.FieldTypeList, containers.Type)
	affinity := findField(t, spec.SubFields, "affinity")
	assert.Equal(t, schema.FieldTypeObject, affinity.Type)
	assert.NotEmpty(t, affinity.SubFields, "spec.affinity must be resolved to its nested fields")

	status := findField(t, fields, "status")
	assert.NotEmpty(t, status.SubFields, "status must be enriched with nested fields from the embedded swagger snapshot")
	assert.Equal(t, schema.FieldTypeString, findField(t, status.SubFields, "phase").Type)
	assert.Equal(t, schema.FieldTypeList, findField(t, status.SubFields, "conditions").Type)
}

// TestStrategicSchemaProvider_UnknownResourceUnaffected verifies that a
// CRD-style resource not covered by the embedded swagger snapshot falls back
// to the default baseline unchanged: spec/status remain present but empty.
func TestStrategicSchemaProvider_UnknownResourceUnaffected(t *testing.T) {
	provider := newStrategicSchemaProvider(context.Background(), "", nil, nil)

	fields, err := provider.Provide(context.Background(), k8sschema.GroupVersionResource{Group: "acme.example.com", Version: "v1", Resource: "widgets"})
	require.NoError(t, err)

	spec := findField(t, fields, "spec")
	assert.Equal(t, schema.FieldTypeObject, spec.Type)
	assert.Empty(t, spec.SubFields, "spec is unaffected by the embedded snapshot for resources it doesn't cover")

	status := findField(t, fields, "status")
	assert.Equal(t, schema.FieldTypeObject, status.Type)
	assert.Empty(t, status.SubFields, "status is unaffected by the embedded snapshot for resources it doesn't cover")
}

// findField returns the named field from fs or fails the test.
func findField(t *testing.T, fs []schema.Field, name string) schema.Field {
	t.Helper()
	for _, f := range fs {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("field %q not found among %v", name, fs)
	return schema.Field{}
}
