package octosql

// Uncomment the code below to test octosql's native "[]" list-indexing operator
// for later

// arrayIndexFakeDS serves a single pod whose spec.volumes is a list with two
// elements, driving the full query pipeline so octosql's native "[]"
// list-indexing operator is exercised end-to-end via "spec->volumes[0]".
// type arrayIndexFakeDS struct{}

// func (arrayIndexFakeDS) Resolve(_ context.Context, _ string) (k8sport.Resource, error) {
// 	return k8sport.Resource{Name: "pods", Version: "v1", Namespaced: true}, nil
// }
// func (arrayIndexFakeDS) Resources(context.Context) ([]k8sport.Resource, error) { return nil, nil }
// func (arrayIndexFakeDS) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
// 	return []schema.Field{
// 		{Name: "name", Type: schema.FieldTypeString},
// 		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
// 			{Name: "volumes", Type: schema.FieldTypeList},
// 		}},
// 	}, nil
// }
// func (arrayIndexFakeDS) List(_ context.Context, _ k8sport.Resource, _ k8sport.ListOptions, fn func([]map[string]any) error) error {
// 	return fn([]map[string]any{
// 		{
// 			"metadata": map[string]any{"name": "nginx"},
// 			"spec": map[string]any{
// 				"volumes": []any{
// 					map[string]any{"name": "config"},
// 					map[string]any{"name": "data"},
// 				},
// 			},
// 		},
// 	})
// }

// TestArrayIndex_StructFieldThenIndex drives the full octosql pipeline and
// asserts "spec->volumes[0]" resolves via octosql's native "[]" list-indexing
// operator (struct field access -> list -> element).
// func TestArrayIndex_StructFieldThenIndex(t *testing.T) {
// 	eng := New(portsql.Config{Output: "json"}, arrayIndexFakeDS{})
// 	var buf strings.Builder
// 	err := eng.Execute(context.Background(),
// 		portsql.Query{SQL: "SELECT spec->volumes[0] AS first_volume FROM pods"},
// 		&buf)
// 	require.NoError(t, err, "execute")

// 	var rows []map[string]any
// 	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "output is JSON: %s", buf.String())
// 	require.Len(t, rows, 1)

// 	first, ok := rows[0]["first_volume"].(map[string]any)
// 	require.True(t, ok, "spec->volumes[0] renders as a JSON object, got %T", rows[0]["first_volume"])
// 	assert.Equal(t, "config", first["name"])
// }
