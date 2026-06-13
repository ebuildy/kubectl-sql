package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

func strSchema() spec.Schema {
	return spec.Schema{SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}}
}

func TestOpenAPITypeToFieldType_MapVsStruct(t *testing.T) {
	// map[string]string: type=object, additionalProperties set, no properties.
	mapSchema := &spec.Schema{SchemaProps: spec.SchemaProps{
		Type:                 spec.StringOrArray{"object"},
		AdditionalProperties: &spec.SchemaOrBool{Allows: true, Schema: &spec.Schema{SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}}},
	}}
	assert.Equal(t, schema.FieldTypeMap, openAPITypeToFieldType(mapSchema),
		"object with additionalProperties and no properties is a map")

	// Fixed struct: type=object with named properties.
	structSchema := &spec.Schema{SchemaProps: spec.SchemaProps{
		Type:       spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{"name": strSchema()},
	}}
	assert.Equal(t, schema.FieldTypeObject, openAPITypeToFieldType(structSchema),
		"object with properties is a struct")

	// Structural $ref (no explicit type) stays a struct.
	refSchema := &spec.Schema{SchemaProps: spec.SchemaProps{Ref: mustRef(t)}}
	assert.Equal(t, schema.FieldTypeObject, openAPITypeToFieldType(refSchema),
		"$ref object is a struct")
}

func mustRef(t *testing.T) spec.Ref {
	t.Helper()
	r, err := spec.NewRef("#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDefaultSchema_LabelsAreMaps(t *testing.T) {
	fields, err := newDefaultSchemaProvider().Provide(context.Background(), k8sschema.GroupVersionResource{Resource: "pods"})
	if err != nil {
		t.Fatal(err)
	}

	byName := func(fs []schema.Field, name string) (schema.Field, bool) {
		for _, f := range fs {
			if f.Name == name {
				return f, true
			}
		}
		return schema.Field{}, false
	}

	labels, ok := byName(fields, "labels")
	assert.True(t, ok)
	assert.Equal(t, schema.FieldTypeMap, labels.Type, "top-level labels is a map")

	annotations, ok := byName(fields, "annotations")
	assert.True(t, ok)
	assert.Equal(t, schema.FieldTypeMap, annotations.Type, "top-level annotations is a map")

	metadata, ok := byName(fields, "metadata")
	assert.True(t, ok)
	assert.Equal(t, schema.FieldTypeObject, metadata.Type, "metadata is a struct")
	mLabels, ok := byName(metadata.SubFields, "labels")
	assert.True(t, ok)
	assert.Equal(t, schema.FieldTypeMap, mLabels.Type, "metadata.labels is a map")

	specField, ok := byName(fields, "spec")
	assert.True(t, ok)
	assert.Equal(t, schema.FieldTypeObject, specField.Type, "spec is a struct")
}

func TestMergeSchemas_MapKindNotDowngradedBySample(t *testing.T) {
	// Default baseline: labels is a map (no keys yet).
	root := &schema.Field{Name: "root", Type: schema.FieldTypeObject, SubFields: []schema.Field{
		{Name: "labels", Type: schema.FieldTypeMap},
	}}
	// A later sample sees labels as an object with keys.
	sample := []schema.Field{
		{Name: "labels", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "app", Type: schema.FieldTypeString},
		}},
	}

	if err := mergeSchemas(root, sample); err != nil {
		t.Fatal(err)
	}

	labels := root.SubFields[0]
	assert.Equal(t, schema.FieldTypeMap, labels.Type, "kind stays map (not downgraded to object)")
	assert.Empty(t, labels.SubFields, "sample keys (e.g. \"app\") are not added as per-key map subfields")
}
