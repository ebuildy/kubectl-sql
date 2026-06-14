package k8s

import (
	"bytes"
	"context"
	"encoding/gob"
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

func TestSwaggerSchemaProvider_ListOfObjects(t *testing.T) {
	p := newSwaggerSchemaProvider(context.Background())

	fields, err := p.Provide(context.Background(), k8sschema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"})
	require.NoError(t, err)
	require.NotEmpty(t, fields)

	// Guaranteed columns are prepended (8.6: element schema follows the prefix).
	assert.Equal(t, "name", fields[0].Name)
	assert.Equal(t, "namespace", fields[1].Name)

	var spec *schema.Field
	for i := range fields {
		switch fields[i].Name {
		case "spec":
			spec = &fields[i]
		}
	}
	require.NotNil(t, spec)

	// spec->containers is a list whose SubFields carry the Container element schema.
	containers := spec.Child("containers")
	require.NotNil(t, containers)
	assert.Equal(t, schema.FieldTypeList, containers.Type)
	require.NotEmpty(t, containers.SubFields, "containers must carry its element (Container) schema")
	require.NotNil(t, containers.Child("name"), "Container element has a name field")
	require.NotNil(t, containers.Child("image"), "Container element has an image field")

	// ports is a nested object-element list inside the Container element (8.7 depth).
	ports := containers.Child("ports")
	require.NotNil(t, ports)
	assert.Equal(t, schema.FieldTypeList, ports.Type)
	require.NotEmpty(t, ports.SubFields, "nested ports list carries its ContainerPort element schema")
}

// TestSwaggerSchemaProvider_GobRoundTripListElements verifies that a field tree
// with list element SubFields nested several levels deep survives gob
// encode/decode unchanged (8.7), and that scalar-element lists round-trip with
// nil SubFields and no spurious children (8.8).
func TestSwaggerSchemaProvider_GobRoundTripListElements(t *testing.T) {
	original := []schema.Field{
		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			// Object-element list nested several levels deep.
			{Name: "containers", Type: schema.FieldTypeList, SubFields: []schema.Field{
				{Name: "name", Type: schema.FieldTypeString},
				{Name: "ports", Type: schema.FieldTypeList, SubFields: []schema.Field{
					{Name: "containerPort", Type: schema.FieldTypeInt},
				}},
				// Scalar-element list keeps nil SubFields (8.8).
				{Name: "command", Type: schema.FieldTypeList},
			}},
		}},
	}

	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(original))
	var decoded []schema.Field
	require.NoError(t, gob.NewDecoder(&buf).Decode(&decoded))

	assert.Equal(t, original, decoded, "list element SubFields must survive gob round-trip")

	spec := (&schema.Field{SubFields: decoded}).Child("spec")
	require.NotNil(t, spec)
	containers := spec.Child("containers")
	require.NotNil(t, containers)
	assert.NotEmpty(t, containers.SubFields)
	command := containers.Child("command")
	require.NotNil(t, command)
	assert.Nil(t, command.SubFields, "scalar-element list round-trips with nil SubFields")
}
