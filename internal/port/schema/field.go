// Package schema holds the library-free column model used across the ports.
// It contains no Kubernetes (or any datasource library) imports so it can be
// referenced by both ports and adapters without coupling.
package schema

import (
	"encoding/json"
	"strconv"
	"strings"
)

// SubFieldsAtPath walks fields following path (a chain of -> separated segments,
// e.g. ["spec", "containers", "0"]) and returns the SubFields at that depth, or
// nil if any segment cannot be resolved. Numeric segments (list indices) stay on
// the current field set, since inferred list elements share one SubFields set
// regardless of index. Used by both Tab completion (arrow-chain completion) and
// the SQL engine's typo-correction (scoping nested field candidates to the
// parent struct).
func SubFieldsAtPath(fields []Field, path []string) []Field {
	for _, seg := range path {
		if _, err := strconv.Atoi(seg); err == nil {
			// Array index: stay on the current field set (list elements all
			// share the same SubFields).
			continue
		}
		var next []Field
		found := false
		for _, f := range fields {
			if strings.EqualFold(f.Name, seg) {
				next = f.SubFields
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		fields = next
	}
	return fields
}

// FieldType describes the inferred type of a resource column.
type FieldType string

const (
	FieldTypeString FieldType = "string"
	FieldTypeInt    FieldType = "int"
	FieldTypeFloat  FieldType = "float"
	FieldTypeBool   FieldType = "bool"
	FieldTypeObject FieldType = "object" // fixed-schema struct (metadata, spec, status) → octosql Struct
	FieldTypeMap    FieldType = "map"    // open-ended map[string]T (labels, annotations) → octosql Struct over sample keys
	FieldTypeList   FieldType = "list"   // slices → octosql List (JSON-string elements)
)

// IsObjectLike reports whether the field type carries named subfields and nests
// recursively — i.e. a fixed-schema struct OR an open-ended map. Both materialize
// as an octosql Struct; they differ only in how columns/keys are presented.
func (t FieldType) IsObjectLike() bool {
	return t == FieldTypeObject || t == FieldTypeMap
}

// Field represents a single inferred column.
// Name is the SQL-safe column name (dots replaced with underscores).
//
// SubFields carries the nested schema and its meaning depends on Type:
//   - FieldTypeObject: SubFields are the struct's own fields (e.g. metadata, spec).
//   - FieldTypeList: SubFields describe the schema of each list ELEMENT (not
//     subfields of the list itself). A list whose element is an object (e.g.
//     spec->containers, whose element is a Container) carries that element's
//     fields here, so list[index]->field resolves. A list with no resolvable
//     element object schema (scalar/[]string, map elements) leaves SubFields nil.
//   - FieldTypeMap: SubFields are unused (maps are open-ended key/value).
type Field struct {
	Name      string
	Type      FieldType
	SubFields []Field
}

// Get sub field by name
func (f Field) Child(n string) *Field {
	for _, ff := range f.SubFields {
		if ff.Name == n {
			return &ff
		}
	}

	return nil
}

// ignoredFieldNames are server-managed metadata fields that add noise to query
// output and schema/autocomplete without being useful to query. They are dropped
// wherever subfields are built (they live under metadata, e.g. metadata->managedFields).
var ignoredFieldNames = map[string]bool{
	"managedFields":   true,
	"resourceVersion": true,
	"generation":      true,
}

// isIgnoredField reports whether a field name should be omitted from inference.
func isIgnoredField(name string) bool {
	return ignoredFieldNames[name]
}

// IsIgnoredField reports whether a field name is a server-managed field that
// should be omitted from inference. Exported for adapters that build fields.
func IsIgnoredField(name string) bool { return isIgnoredField(name) }

// IsGuaranteedName reports whether name is one of the always-present columns
// (name, namespace) that inferrers prepend.
func IsGuaranteedName(name string) bool { return guaranteedNames[name] }

// GuaranteedFields returns a copy of the always-present field list (name, namespace).
func GuaranteedFields() []Field {
	out := make([]Field, len(guaranteedFields))
	copy(out, guaranteedFields)
	return out
}

// fieldJSON is the JSON shape used by MarshalSubFieldsJSON: only the parts of
// Field meaningful to a reader of DESCRIBE TABLE's SCHEMA column. Path (an
// internal resolver detail) is omitted.
type fieldJSON struct {
	Name      string      `json:"name"`
	Type      FieldType   `json:"type"`
	SubFields []fieldJSON `json:"subFields,omitempty"`
}

func toFieldJSON(fields []Field) []fieldJSON {
	out := make([]fieldJSON, len(fields))
	for i, f := range fields {
		out[i] = fieldJSON{
			Name:      f.Name,
			Type:      f.Type,
			SubFields: toFieldJSON(f.SubFields),
		}
	}
	return out
}

// MarshalSubFieldsJSON recursively encodes fields (typically a Field's
// SubFields) as pretty-printed JSON, retaining only Name, Type, and SubFields
// at every depth.
func MarshalSubFieldsJSON(fields []Field) (string, error) {
	b, err := json.MarshalIndent(toFieldJSON(fields), "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// LimitDepth returns a copy of fields with SubFields truncated beyond
// maxDepth levels, for callers that need to bound how deeply nested schemas
// are rendered (e.g. DESCRIBE TABLE's SCHEMA column).
func LimitDepth(fields []Field, maxDepth int) []Field {
	if maxDepth <= 0 || len(fields) == 0 {
		return nil
	}
	out := make([]Field, len(fields))
	for i, f := range fields {
		out[i] = Field{
			Name:      f.Name,
			Type:      f.Type,
			SubFields: LimitDepth(f.SubFields, maxDepth-1),
		}
	}
	return out
}
