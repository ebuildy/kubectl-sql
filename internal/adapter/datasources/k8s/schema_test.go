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

	fields, err := c.InferFields(context.Background(), k8sschema.GroupVersionResource{Resource: "pods"})
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
	expected := `[{"Name":"metadata","Path":"","Type":"object","SubFields":[{"Name":"name","Path":"","Type":"string","SubFields":null},{"Name":"namespace","Path":"","Type":"string","SubFields":null},{"Name":"labels","Path":"","Type":"object","SubFields":null}]},{"Name":"status","Path":"","Type":"object","SubFields":null},{"Name":"spec","Path":"","Type":"object","SubFields":[{"Name":"name","Path":"","Type":"string","SubFields":null}]}]`

	assert.JSONEq(t, expected, string(resultAsJSON), "unexpected merged schema")

	err = mergeSchemas(root, []schema.Field{
		{Name: "metadata", Type: schema.FieldTypeString},
	})
	assert.Error(t, err, "expected error merging schemas")
}
