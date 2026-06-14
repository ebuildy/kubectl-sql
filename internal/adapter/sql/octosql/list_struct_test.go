package octosql

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cube2222/octosql/octosql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// 9.1 — fieldToOctoType maps list element schema to the element octosql type.
func TestFieldToOctoType_ListElement(t *testing.T) {
	// Object-element list → List<Struct{...}>.
	objList := fieldToOctoType(schema.Field{
		Name: "containers", Type: schema.FieldTypeList,
		SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "image", Type: schema.FieldTypeString},
		},
	})
	require.Equal(t, octosql.TypeIDList, objList.TypeID)
	require.NotNil(t, objList.List.Element)
	assert.Equal(t, octosql.TypeIDStruct, objList.List.Element.TypeID)
	require.Len(t, objList.List.Element.Struct.Fields, 2)
	assert.Equal(t, "name", objList.List.Element.Struct.Fields[0].Name)

	// Scalar-element list → List<String>.
	scalarList := fieldToOctoType(schema.Field{Name: "command", Type: schema.FieldTypeList})
	require.Equal(t, octosql.TypeIDList, scalarList.TypeID)
	require.NotNil(t, scalarList.List.Element)
	assert.Equal(t, octosql.TypeIDString, scalarList.List.Element.TypeID)

	// Map → List<Any> (unchanged).
	mapType := fieldToOctoType(schema.Field{Name: "labels", Type: schema.FieldTypeMap})
	require.Equal(t, octosql.TypeIDList, mapType.TypeID)
	require.NotNil(t, mapType.List.Element)
	assert.Equal(t, octosql.TypeIDAny, mapType.List.Element.TypeID)
}

// 9.2 — value materialization: a list-of-object field resolves to a List of
// Struct values with positional fields matching subfields (missing keys → NULL).
func TestAnyToListValue_StructElements(t *testing.T) {
	subFields := []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "image", Type: schema.FieldTypeString},
	}
	raw := []interface{}{
		map[string]interface{}{"name": "nginx", "image": "nginx:1.25"},
		map[string]interface{}{"name": "sidecar"}, // missing image → NULL
	}

	v := anyToListValue(raw, subFields)
	require.Equal(t, octosql.TypeIDList, v.TypeID)
	require.Len(t, v.List, 2)

	first := v.List[0]
	require.Equal(t, octosql.TypeIDStruct, first.TypeID)
	require.Len(t, first.Struct, 2)
	assert.Equal(t, "nginx", first.Struct[0].Str)
	assert.Equal(t, "nginx:1.25", first.Struct[1].Str)

	second := v.List[1]
	require.Equal(t, octosql.TypeIDStruct, second.TypeID)
	require.Len(t, second.Struct, 2, "struct arity matches declared element type")
	assert.Equal(t, "sidecar", second.Struct[0].Str)
	assert.Equal(t, octosql.TypeIDNull, second.Struct[1].TypeID, "missing key → NULL")
}

// listStructFakeDS serves two pods each with spec.containers (object-element list)
// and a scalar-element command list.
type listStructFakeDS struct{}

func (listStructFakeDS) Resolve(_ context.Context, _ string) (k8sport.Resource, error) {
	return k8sport.Resource{Name: "pods", Version: "v1", Namespaced: true}, nil
}
func (listStructFakeDS) Resources(context.Context) ([]k8sport.Resource, error) { return nil, nil }
func (listStructFakeDS) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
	return []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "containers", Type: schema.FieldTypeList, SubFields: []schema.Field{
				{Name: "name", Type: schema.FieldTypeString},
				{Name: "image", Type: schema.FieldTypeString},
				{Name: "command", Type: schema.FieldTypeList}, // scalar-element list
			}},
		}},
	}, nil
}
func (listStructFakeDS) List(_ context.Context, _ k8sport.Resource, _ k8sport.ListOptions, fn func([]map[string]any) error) error {
	return fn([]map[string]any{
		{
			"metadata": map[string]any{"name": "web"},
			"spec": map[string]any{"containers": []interface{}{
				map[string]interface{}{"name": "nginx", "image": "nginx:1.25", "command": []interface{}{"nginx", "-g"}},
				map[string]interface{}{"name": "sidecar", "image": "envoy:1.30"},
			}},
		},
		{
			"metadata": map[string]any{"name": "cache"},
			"spec": map[string]any{"containers": []interface{}{
				map[string]interface{}{"name": "redis", "image": "redis:7"},
			}},
		},
	})
}

// 9.4 — SELECT name, spec->containers[0]->name type-checks, executes, projects
// the first container name.
func TestQuery_ListElementFieldAccess(t *testing.T) {
	eng := New(portsql.Config{Output: "json"}, listStructFakeDS{})
	var buf strings.Builder
	err := eng.Execute(context.Background(),
		portsql.Query{SQL: "SELECT name AS name, spec->containers[0]->name AS c0 FROM pods"},
		&buf)
	require.NoError(t, err, "execute: %s", buf.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "JSON: %s", buf.String())
	require.Len(t, rows, 2)
	assert.Equal(t, "nginx", rows[0]["c0"])
	assert.Equal(t, "redis", rows[1]["c0"])
}

// 9.5 — out-of-range index yields NULL, exit 0.
func TestQuery_ListElementOutOfRange(t *testing.T) {
	eng := New(portsql.Config{Output: "json"}, listStructFakeDS{})
	var buf strings.Builder
	err := eng.Execute(context.Background(),
		portsql.Query{SQL: "SELECT name AS name, spec->containers[99]->name AS c FROM pods"},
		&buf)
	require.NoError(t, err, "execute: %s", buf.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "JSON: %s", buf.String())
	require.Len(t, rows, 2)
	assert.Nil(t, rows[0]["c"], "out-of-range index → NULL")
	assert.Nil(t, rows[1]["c"], "out-of-range index → NULL")
}

// 9.6 — list element field usable in WHERE.
func TestQuery_ListElementInWhere(t *testing.T) {
	eng := New(portsql.Config{Output: "json"}, listStructFakeDS{})
	var buf strings.Builder
	err := eng.Execute(context.Background(),
		portsql.Query{SQL: "SELECT name AS name FROM pods WHERE spec->containers[0]->image = 'redis:7'"},
		&buf)
	require.NoError(t, err, "execute: %s", buf.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "JSON: %s", buf.String())
	require.Len(t, rows, 1)
	assert.Equal(t, "cache", rows[0]["name"])
}

// 9.3 / 9.7 — JSON output renders List<Struct> as an array of named-key objects
// (with nested scalar-element command list unchanged as a JSON array).
func TestQuery_ListStructJSONOutput(t *testing.T) {
	eng := New(portsql.Config{Output: "json"}, listStructFakeDS{})
	var buf strings.Builder
	err := eng.Execute(context.Background(),
		portsql.Query{SQL: "SELECT spec->containers AS containers FROM pods"},
		&buf)
	require.NoError(t, err, "execute: %s", buf.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "JSON: %s", buf.String())
	require.Len(t, rows, 2)

	containers, ok := rows[0]["containers"].([]interface{})
	require.True(t, ok, "containers should be an array, got %T", rows[0]["containers"])
	require.Len(t, containers, 2)

	first, ok := containers[0].(map[string]interface{})
	require.True(t, ok, "element should be a named-key object, got %T", containers[0])
	assert.Equal(t, "nginx", first["name"])
	assert.Equal(t, "nginx:1.25", first["image"])
	// 9.7 — scalar-element list inside the struct stays a JSON array of strings.
	cmd, ok := first["command"].([]interface{})
	require.True(t, ok, "command should be an array, got %T", first["command"])
	assert.Equal(t, []interface{}{"nginx", "-g"}, cmd)
}

// Indexing a typed list (spec->containers[0]) yields a single element struct.
// JSON output must render it as a named-key object, not octosql's positional form.
func TestQuery_ListElementIndexJSONOutput(t *testing.T) {
	eng := New(portsql.Config{Output: "json"}, listStructFakeDS{})
	var buf strings.Builder
	err := eng.Execute(context.Background(),
		portsql.Query{SQL: "SELECT spec->containers[0] AS c0 FROM pods"},
		&buf)
	require.NoError(t, err, "execute: %s", buf.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "JSON: %s", buf.String())
	require.Len(t, rows, 2)

	c0, ok := rows[0]["c0"].(map[string]interface{})
	require.True(t, ok, "indexed element should be a named-key object, got %T: %v", rows[0]["c0"], rows[0]["c0"])
	assert.Equal(t, "nginx", c0["name"])
	assert.Equal(t, "nginx:1.25", c0["image"])
}

// Indexing a typed list yields a nullable struct type (Null | Struct); table
// output with beautify must still render it as a pretty YAML object.
func TestRenderTable_ListElementIndexYAML(t *testing.T) {
	structType := structTypeFromFields([]schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "image", Type: schema.FieldTypeString},
	})
	nullable := octosql.Type{
		TypeID: octosql.TypeIDUnion,
		Union:  struct{ Alternatives []octosql.Type }{Alternatives: []octosql.Type{octosql.Null, structType}},
	}
	cell := octosql.NewStruct([]octosql.Value{octosql.NewString("nginx"), octosql.NewString("nginx:1.25")})

	require.True(t, rendersAsJSON(cell, nullable), "nullable struct cell must render as an object")
	out := valueToStringTyped(cell, nullable, true)
	assert.Contains(t, out, "name: nginx", "pretty YAML object with resolved field names")
	assert.Contains(t, out, "image: nginx:1.25")
}

// 9.3 — table output renders a List<Struct> cell as a YAML sequence of named-key
// mappings (default beautify format).
func TestRenderTable_ListStructYAML(t *testing.T) {
	elemType := structTypeFromFields([]schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "image", Type: schema.FieldTypeString},
	})
	listType := octosql.Type{
		TypeID: octosql.TypeIDList,
		List:   struct{ Element *octosql.Type }{Element: &elemType},
	}
	cell := octosql.NewList([]octosql.Value{
		octosql.NewStruct([]octosql.Value{octosql.NewString("nginx"), octosql.NewString("nginx:1.25")}),
	})

	out := valueToStringTyped(cell, listType, true)
	assert.Contains(t, out, "name: nginx", "YAML sequence of named-key mappings")
	assert.Contains(t, out, "image: nginx:1.25")
	assert.NotContains(t, out, "\\\"", "elements are not escaped JSON strings")
}
