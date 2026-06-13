package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// fieldByName finds a field by name, failing the test if absent.
func fieldByName(t *testing.T, fields []schema.Field, name string) schema.Field {
	t.Helper()
	for _, f := range fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("field %q not found among %v", name, fields)
	return schema.Field{}
}

func TestDefToFields_Pod(t *testing.T) {
	doc := loadFixture(t)

	fields := defToFields(doc.Definitions, "test.Pod")

	apiVersion := fieldByName(t, fields, "apiVersion")
	assert.Equal(t, schema.FieldTypeString, apiVersion.Type)

	metadata := fieldByName(t, fields, "metadata")
	assert.Equal(t, schema.FieldTypeObject, metadata.Type)
	assert.Equal(t, schema.FieldTypeMap, fieldByName(t, metadata.SubFields, "labels").Type)
	assert.Equal(t, schema.FieldTypeMap, fieldByName(t, metadata.SubFields, "annotations").Type)

	specField := fieldByName(t, fields, "spec")
	require.Equal(t, schema.FieldTypeObject, specField.Type)
	assert.Equal(t, schema.FieldTypeString, fieldByName(t, specField.SubFields, "nodeName").Type)

	containers := fieldByName(t, specField.SubFields, "containers")
	assert.Equal(t, schema.FieldTypeList, containers.Type)
	assert.Nil(t, containers.SubFields, "array fields remain leaves with no SubFields")

	status := fieldByName(t, fields, "status")
	require.Equal(t, schema.FieldTypeObject, status.Type)
	assert.Equal(t, schema.FieldTypeString, fieldByName(t, status.SubFields, "phase").Type)
	assert.Equal(t, schema.FieldTypeList, fieldByName(t, status.SubFields, "conditions").Type)
}

func TestDefToFields_CycleGuard(t *testing.T) {
	doc := loadFixture(t)

	fields := defToFields(doc.Definitions, "test.Widget")

	specField := fieldByName(t, fields, "spec")
	require.Equal(t, schema.FieldTypeObject, specField.Type)

	validation := fieldByName(t, specField.SubFields, "validation")
	require.Equal(t, schema.FieldTypeObject, validation.Type)

	not := fieldByName(t, validation.SubFields, "not")
	assert.Equal(t, schema.FieldTypeObject, not.Type)
	assert.Nil(t, not.SubFields, "self-referential $ref must be truncated to a childless object")
}

func TestDefToFields_MaxDepthCap(t *testing.T) {
	// Build a chain of definitions deeper than maxDepth, each wrapping the next
	// via a single "next" $ref property, terminating in a scalar field.
	defs := spec.Definitions{}
	const chainLen = maxDepth + 4
	for i := 0; i < chainLen; i++ {
		name := chainDefName(i)
		props := map[string]spec.Schema{
			"leaf": *spec.StringProperty(),
		}
		if i+1 < chainLen {
			props["next"] = *spec.RefProperty("#/definitions/" + chainDefName(i+1))
		}
		defs[name] = spec.Schema{SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"object"}, Properties: props}}
	}

	fields := defToFields(defs, chainDefName(0))

	// Walk "next" until we either run out of SubFields or hit maxDepth.
	depth := 0
	cur := fields
	for {
		next := fieldByName(t, cur, "next")
		depth++
		if next.SubFields == nil {
			break
		}
		cur = next.SubFields
		require.Less(t, depth, chainLen, "recursion did not terminate")
	}

	assert.Equal(t, maxDepth+1, depth, "recursion must be truncated one hop past maxDepth")
}

func chainDefName(i int) string {
	return "test.Chain" + string(rune('A'+i))
}
