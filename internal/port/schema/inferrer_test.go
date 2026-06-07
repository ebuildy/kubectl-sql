package schema

import (
	"strings"
	"testing"
)

func fieldNames(fields []Field) []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	return names
}

func findField(fields []Field, name string) (Field, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

func TestInferFields_WithSample(t *testing.T) {
	obj := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": "test"},
		"spec":       map[string]interface{}{"containers": []interface{}{}},
		"status":     map[string]interface{}{"phase": "Running"},
	}
	fields := InferFields(obj)
	if fields == nil {
		t.Fatal("expected non-nil fields")
	}

	for _, required := range []string{"name", "namespace", "spec", "status"} {
		if _, ok := findField(fields, required); !ok {
			t.Errorf("missing expected field %q, got: %v", required, fieldNames(fields))
		}
	}

	if f, ok := findField(fields, "spec"); ok {
		if f.Type != FieldTypeObject {
			t.Errorf("spec: expected JSON type, got %s", f.Type)
		}
	}
	if f, ok := findField(fields, "apiVersion"); ok {
		if f.Type != FieldTypeString {
			t.Errorf("apiVersion: expected string type, got %s", f.Type)
		}
	}
}

func TestInferFields_Empty(t *testing.T) {
	if InferFields(nil) != nil {
		t.Error("nil obj should return nil")
	}
	if InferFields(map[string]interface{}{}) != nil {
		t.Error("empty obj should return nil")
	}
}

func TestInferFields_GuaranteedFields(t *testing.T) {
	// Object with no name/namespace/raw keys
	obj := map[string]interface{}{
		"spec": "something",
	}
	fields := InferFields(obj)
	for _, name := range []string{"name", "namespace"} {
		f, ok := findField(fields, name)
		if !ok {
			t.Errorf("guaranteed field %q missing", name)
			continue
		}
		if f.Type != FieldTypeString {
			t.Errorf("%s field should be string type, got %s", name, f.Type)
		}
	}
	// Guaranteed fields must be first
	if fields[0].Name != "name" || fields[1].Name != "namespace" {
		t.Errorf("guaranteed fields must be first two, got: %v", fieldNames(fields))
	}
}

func TestInferFields_NoDuplicates(t *testing.T) {
	obj := map[string]interface{}{
		"name":      "pod-a",
		"namespace": "default",
		"status":    "running",
	}
	fields := InferFields(obj)
	seen := map[string]int{}
	for _, f := range fields {
		seen[f.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("field %q appears %d times", name, count)
		}
	}
}

func TestTypeOf(t *testing.T) {
	cases := []struct {
		input    interface{}
		expected FieldType
	}{
		{nil, FieldTypeString},
		{"hello", FieldTypeString},
		{true, FieldTypeBool},
		{false, FieldTypeBool},
		{int64(42), FieldTypeInt},
		{float64(42), FieldTypeInt},
		{float64(3.14), FieldTypeFloat},
		{map[string]interface{}{}, FieldTypeObject},
		{[]interface{}{}, FieldTypeList},
	}
	for _, tc := range cases {
		got := typeOf(tc.input)
		if got != tc.expected {
			t.Errorf("typeOf(%T %v) = %s, want %s", tc.input, tc.input, got, tc.expected)
		}
	}
}

func TestWalkObject_NestedMap(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{"app": "nginx", "env": "test"},
			"name":   "pod-a",
		},
	}
	fields := walkObject(obj)
	f, ok := findField(fields, "metadata")
	if !ok {
		t.Fatal("expected metadata field")
	}
	if f.Type != FieldTypeObject {
		t.Errorf("metadata: expected FieldTypeObject, got %s", f.Type)
	}
	if len(f.SubFields) == 0 {
		t.Error("metadata: expected non-empty SubFields")
	}
	found := false
	for _, sf := range f.SubFields {
		if sf.Name == "labels" {
			found = true
		}
	}
	if !found {
		t.Error("metadata SubFields: expected labels subfield")
	}
	// No flattened alias fields should be emitted.
	if _, aliasFound := findField(fields, "metadata_labels"); aliasFound {
		t.Error("walkObject must not emit flattened alias field metadata_labels")
	}
	if _, aliasFound := findField(fields, "metadata_labels_app"); aliasFound {
		t.Error("walkObject must not emit flattened alias field metadata_labels_app")
	}
}

func TestWalkObject_IgnoresServerManagedMetadata(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   "pod-a",
			"labels": map[string]interface{}{"app": "nginx"},
			"managedFields": []interface{}{
				map[string]interface{}{"manager": "kubelet", "apiVersion": "v1"},
			},
			"resourceVersion": "12345",
			"generation":      int64(3),
		},
	}
	fields := walkObject(obj)
	meta, ok := findField(fields, "metadata")
	if !ok {
		t.Fatal("expected metadata field")
	}
	for _, sf := range meta.SubFields {
		if isIgnoredField(sf.Name) {
			t.Errorf("metadata SubFields must not include server-managed %q, got: %v",
				sf.Name, fieldNames(meta.SubFields))
		}
	}
	// Useful fields are still present.
	if _, ok := findField(meta.SubFields, "labels"); !ok {
		t.Errorf("expected labels subfield to remain, got: %v", fieldNames(meta.SubFields))
	}
	// No flattened slice-index columns for managedFields, e.g.
	// metadata_managedFields_0_apiVersion.
	for _, f := range fields {
		if strings.HasPrefix(f.Name, "metadata_managedFields") {
			t.Errorf("must not emit flattened managedFields column %q", f.Name)
		}
	}
}

func TestWalkObject_Slice(t *testing.T) {
	obj := map[string]interface{}{
		"items": []interface{}{"a", "b"},
	}
	fields := walkObject(obj)
	f, ok := findField(fields, "items")
	if !ok {
		t.Fatal("expected items field")
	}
	if f.Type != FieldTypeList {
		t.Errorf("items: expected FieldTypeList, got %s", f.Type)
	}
	if len(f.SubFields) != 0 {
		t.Errorf("items: expected empty SubFields for slice, got %v", f.SubFields)
	}
}
