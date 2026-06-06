package schema

import (
	"fmt"
	"sort"
)

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
// Top-level slice values produce per-element index fields (e.g. volumes_0, volumes_1)
// with Path set to the bracket-notation path (e.g. volumes[0]).
// No flattened dot-alias columns are emitted — nested map access uses the -> operator.
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
		if guaranteedNames[key] || isIgnoredField(key) {
			continue
		}
		val := obj[key]
		f := Field{Name: key, Type: typeOf(val)}
		switch v := val.(type) {
		case map[string]interface{}:
			f.SubFields = walkSubFields(v)
			// Emit top-level index columns for any slice values one level down.
			// e.g. spec.volumes → spec_volumes_0, spec_volumes_0_configMap, …
			fields = append(fields, walkSliceChildren(key, v)...)
		case []interface{}:
			// Direct top-level slice: emit per-index columns immediately.
			fields = append(fields, walkSliceIndexFields(key, v)...)
		}
		fields = append(fields, f)
	}
	return fields
}

// walkSliceChildren walks a map one level down looking for slice values and
// emits flattened index columns for each (parentKey_childKey_N, etc.).
func walkSliceChildren(parentKey string, obj map[string]interface{}) []Field {
	var fields []Field
	subKeys := make([]string, 0, len(obj))
	for k := range obj {
		subKeys = append(subKeys, k)
	}
	sort.Strings(subKeys)
	for _, subKey := range subKeys {
		if isIgnoredField(subKey) {
			continue
		}
		v, ok := obj[subKey].([]interface{})
		if !ok {
			continue
		}
		prefix := parentKey + "_" + subKey
		pathPrefix := parentKey + "." + subKey
		fields = append(fields, walkSliceIndexFields2(prefix, pathPrefix, v)...)
	}
	return fields
}

// walkSliceIndexFields emits index fields for a top-level slice.
func walkSliceIndexFields(key string, slice []interface{}) []Field {
	return walkSliceIndexFields2(key, key, slice)
}

// walkSliceIndexFields2 emits per-element index fields for a slice, using separate
// name prefix (SQL column) and path prefix (resolver path).
func walkSliceIndexFields2(namePrefix, pathPrefix string, slice []interface{}) []Field {
	var fields []Field
	for i, elem := range slice {
		idxName := fmt.Sprintf("%s_%d", namePrefix, i)
		idxPath := fmt.Sprintf("%s[%d]", pathPrefix, i)
		ef := Field{Name: idxName, Path: idxPath, Type: typeOf(elem)}
		if m, ok := elem.(map[string]interface{}); ok {
			ef.SubFields = walkSubFields(m)
			// Emit child columns: spec_volumes_0_configMap, spec_volumes_0_name, …
			childKeys := make([]string, 0, len(m))
			for k := range m {
				childKeys = append(childKeys, k)
			}
			sort.Strings(childKeys)
			for _, childKey := range childKeys {
				childVal := m[childKey]
				cf := Field{
					Name: fmt.Sprintf("%s_%s", idxName, childKey),
					Path: fmt.Sprintf("%s.%s", idxPath, childKey),
					Type: typeOf(childVal),
				}
				if cm, ok := childVal.(map[string]interface{}); ok {
					cf.SubFields = walkLeafFields(cm)
				}
				fields = append(fields, cf)
			}
		}
		fields = append(fields, ef)
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
		if isIgnoredField(k) {
			continue
		}
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
		if isIgnoredField(k) {
			continue
		}
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
