// Package schema holds the library-free column model used across the ports.
// It contains no Kubernetes (or any datasource library) imports so it can be
// referenced by both ports and adapters without coupling.
package schema

import "encoding/json"

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
// Path is the dot-notation resolve path (empty means same as Name).
// SubFields is populated for FieldTypeObject fields inferred from a map value.
type Field struct {
	Name      string
	Path      string
	Type      FieldType
	SubFields []Field
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
// SubFields) as JSON, retaining only Name, Type, and SubFields at every depth.
func MarshalSubFieldsJSON(fields []Field) (string, error) {
	b, err := json.Marshal(toFieldJSON(fields))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
