package schema

import "sort"

// guaranteedFields are always prepended by any inferrer regardless of sample content.
var guaranteedFields = []Field{
	{Name: "name", Type: FieldTypeString},
	{Name: "namespace", Type: FieldTypeString},
}

var guaranteedNames = map[string]bool{
	"name":      true,
	"namespace": true,
}

// walkObject derives a Field slice from an unstructured Kubernetes object map.
// It does NOT prepend guaranteed fields — that is the inferrer's responsibility.
// Returns nil when obj is nil or empty.
// Top-level map values produce a FieldTypeObject field with SubFields one level deep.
// No flattened alias columns are emitted — nested access uses the -> operator.
func walkObject(obj map[string]interface{}) []Field {
	if len(obj) == 0 {
		return nil
	}

	// Sort keys for deterministic field order (required by struct value contract).
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var fields []Field
	for _, key := range keys {
		if guaranteedNames[key] {
			continue
		}
		val := obj[key]
		f := Field{Name: key, Type: typeOf(val)}
		if nested, ok := val.(map[string]interface{}); ok {
			f.SubFields = walkSubFields(nested)
		}
		fields = append(fields, f)
	}
	return fields
}

// walkSubFields builds SubFields for a nested map (two levels deep).
// Each map value that is itself a map gets its own SubFields populated one level down.
func walkSubFields(obj map[string]interface{}) []Field {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fields := make([]Field, 0, len(keys))
	for _, k := range keys {
		v := obj[k]
		f := Field{Name: k, Type: typeOf(v)}
		if nested, ok := v.(map[string]interface{}); ok && len(nested) > 0 {
			f.SubFields = walkLeafFields(nested)
		}
		fields = append(fields, f)
	}
	return fields
}

// walkLeafFields builds a flat Field slice for a map (no further recursion).
func walkLeafFields(obj map[string]interface{}) []Field {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fields := make([]Field, 0, len(keys))
	for _, k := range keys {
		fields = append(fields, Field{Name: k, Type: typeOf(obj[k])})
	}
	return fields
}

// typeOf maps a Go value to its FieldType.
func typeOf(v interface{}) FieldType {
	if v == nil {
		return FieldTypeString
	}
	switch val := v.(type) {
	case bool:
		return FieldTypeBool
	case string:
		return FieldTypeString
	case int, int32, int64:
		return FieldTypeInt
	case float64:
		if val == float64(int64(val)) {
			return FieldTypeInt
		}
		return FieldTypeFloat
	case map[string]interface{}, []interface{}:
		return FieldTypeObject
	default:
		return FieldTypeString
	}
}
