package k8s

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

func TestSchema_Default(t *testing.T) {
	c := newDefaultSchemaProvider()

	fields, err := c.Provide(context.Background(), k8sschema.GroupVersionResource{Resource: "pods"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) == 0 {
		t.Error("expected fields from primary")
	}
}

func TestSchema_MergeSchemas(t *testing.T) {
	dest := []schema.Field{
		{Name: "metadata", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "namespace", Type: schema.FieldTypeString},
		}},
		{Name: "status", Type: schema.FieldTypeObject},
	}
	source := []schema.Field{
		{Name: "metadata", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "labels", Type: schema.FieldTypeObject},
		}},
		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
		}},
		{Name: "status", Type: schema.FieldTypeObject},
	}

	root := &schema.Field{Name: "root", Type: schema.FieldTypeObject, SubFields: dest}

	err := mergeSchemas(root, source)

	assert.Nil(t, err, "unexpected error merging schemas")

	resultAsJSON, _ := json.Marshal(root.SubFields)
	expected := `[{"Name":"metadata","Type":"object","SubFields":[{"Name":"name","Type":"string","SubFields":null},{"Name":"namespace","Type":"string","SubFields":null},{"Name":"labels","Type":"object","SubFields":null}]},{"Name":"status","Type":"object","SubFields":null},{"Name":"spec","Type":"object","SubFields":[{"Name":"name","Type":"string","SubFields":null}]}]`

	assert.JSONEq(t, expected, string(resultAsJSON), "unexpected merged schema")

	// Object-in-dest vs scalar-in-source is enrichment, not conflict: the object form
	// is kept so nested access keeps working.
	err = mergeSchemas(root, []schema.Field{
		{Name: "metadata", Type: schema.FieldTypeString},
	})
	assert.Nil(t, err, "object field should not be downgraded by a scalar source")
	metaIdx := -1
	for i, f := range root.SubFields {
		if f.Name == "metadata" {
			metaIdx = i
		}
	}
	assert.Equal(t, schema.FieldTypeObject, root.SubFields[metaIdx].Type, "metadata should stay an object")

	// A genuine leaf-vs-leaf type conflict is still reported as an error.
	conflict := &schema.Field{Name: "root", Type: schema.FieldTypeObject, SubFields: []schema.Field{
		{Name: "count", Type: schema.FieldTypeInt},
	}}
	err = mergeSchemas(conflict, []schema.Field{
		{Name: "count", Type: schema.FieldTypeString},
	})
	assert.Error(t, err, "expected error on leaf-vs-leaf type conflict")
}
