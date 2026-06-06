package schema

import (
	"context"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemaInferrer is the port for schema inference. All consumers depend only on this interface.
type SchemaInferrer interface {
	InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]Field, error)
}

// FieldType describes the inferred type of a resource column.
type FieldType string

const (
	FieldTypeString FieldType = "string"
	FieldTypeInt    FieldType = "int"
	FieldTypeFloat  FieldType = "float"
	FieldTypeBool   FieldType = "bool"
	FieldTypeObject FieldType = "object" // maps → octosql Struct; slices → JSON string
)

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
