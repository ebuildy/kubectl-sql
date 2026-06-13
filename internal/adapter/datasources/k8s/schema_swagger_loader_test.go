package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

func TestSwaggerSchemaProvider_KnownResource(t *testing.T) {
	p := newSwaggerSchemaProvider(context.Background())

	fields, err := p.Provide(context.Background(), k8sschema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"})
	require.NoError(t, err)
	require.NotEmpty(t, fields)

	// Guaranteed columns are prepended.
	assert.Equal(t, "name", fields[0].Name)
	assert.Equal(t, "namespace", fields[1].Name)

	var spec, status *schema.Field
	for i := range fields {
		switch fields[i].Name {
		case "spec":
			spec = &fields[i]
		case "status":
			status = &fields[i]
		}
	}
	require.NotNil(t, spec, "pods schema must include spec")
	require.NotNil(t, status, "pods schema must include status")
	assert.NotEmpty(t, spec.SubFields, "spec must be resolved to its nested fields")
	assert.NotEmpty(t, status.SubFields, "status must be resolved to its nested fields")
}

func TestSwaggerSchemaProvider_UnknownResource(t *testing.T) {
	p := newSwaggerSchemaProvider(context.Background())

	fields, err := p.Provide(context.Background(), k8sschema.GroupVersionResource{Group: "acme.example.com", Version: "v1", Resource: "widgets"})
	require.NoError(t, err)
	assert.Nil(t, fields)
}

func TestSwaggerSchemaProvider_RepeatedCallsReuseIndex(t *testing.T) {
	p := newSwaggerSchemaProvider(context.Background())
	gvr := k8sschema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

	first, err := p.Provide(context.Background(), gvr)
	require.NoError(t, err)

	second, err := p.Provide(context.Background(), gvr)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}
