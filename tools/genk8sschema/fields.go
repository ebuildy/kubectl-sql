package main

import (
	"sort"
	"strings"

	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// maxDepth caps recursion through $ref-typed object properties, so
// self-referential definitions (e.g. JSONSchemaProps.not -> JSONSchemaProps)
// terminate even without being caught by the cycle guard.
const maxDepth = 8

// defToFields resolves the top-level fields of a swagger definition into a
// []schema.Field tree, recursing into $ref-typed object properties.
func defToFields(defs spec.Definitions, defName string) []schema.Field {
	visiting := map[string]bool{defName: true}
	return propertiesToFields(defs, defs[defName], 0, visiting)
}

// propertiesToFields converts a schema's properties to fields, sorted by name
// for deterministic output. depth == 0 is the resource's own top-level fields,
// where guaranteed columns (name, namespace) are dropped since the loader
// prepends schema.GuaranteedFields(). Server-managed fields (managedFields,
// resourceVersion, generation) are dropped at every level.
func propertiesToFields(defs spec.Definitions, s spec.Schema, depth int, visiting map[string]bool) []schema.Field {
	if len(s.Properties) == 0 {
		return nil
	}

	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		if schema.IsIgnoredField(k) {
			continue
		}
		if depth == 0 && schema.IsGuaranteedName(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fields := make([]schema.Field, 0, len(keys))
	for _, k := range keys {
		prop := s.Properties[k]
		fields = append(fields, schemaToField(defs, k, &prop, depth, visiting))
	}
	return fields
}

// schemaToField converts a single property schema to a field, recursing into
// the referenced (or inline) definition when it is object-typed.
func schemaToField(defs spec.Definitions, name string, s *spec.Schema, depth int, visiting map[string]bool) schema.Field {
	ft, ref := classify(s)
	f := schema.Field{Name: name, Type: ft}

	if ft != schema.FieldTypeObject || depth >= maxDepth {
		return f
	}

	if ref != "" {
		if visiting[ref] {
			return f // cycle: truncate to a childless object
		}
		target, ok := defs[ref]
		if !ok {
			return f
		}
		visiting[ref] = true
		f.SubFields = propertiesToFields(defs, target, depth+1, visiting)
		delete(visiting, ref)
		return f
	}

	// Inline object schema (no $ref): recurse into its own properties.
	f.SubFields = propertiesToFields(defs, *s, depth+1, visiting)
	return f
}

// classify maps an OpenAPI v2 schema to a schema.FieldType, mirroring
// internal/adapter/datasources/k8s/schema_openapi.go's openAPITypeToFieldType.
// When the schema is a $ref to an object-like definition, classify returns
// FieldTypeObject and the referenced definition name so the caller can recurse.
func classify(s *spec.Schema) (ft schema.FieldType, refDefName string) {
	if s == nil {
		return schema.FieldTypeString, ""
	}

	if ref := refName(s); ref != "" {
		return schema.FieldTypeObject, ref
	}

	if len(s.Type) == 0 {
		// Structural schemas (e.g. $ref'd ObjectMeta) carry no explicit "type".
		switch {
		case isMap(s):
			return schema.FieldTypeMap, ""
		case len(s.Properties) > 0 || len(s.AllOf) > 0 || len(s.OneOf) > 0 || len(s.AnyOf) > 0:
			return schema.FieldTypeObject, ""
		default:
			return schema.FieldTypeString, ""
		}
	}

	switch s.Type[0] {
	case "boolean":
		return schema.FieldTypeBool, ""
	case "integer":
		return schema.FieldTypeInt, ""
	case "number":
		return schema.FieldTypeFloat, ""
	case "object":
		if isMap(s) {
			return schema.FieldTypeMap, ""
		}
		return schema.FieldTypeObject, ""
	case "array":
		return schema.FieldTypeList, ""
	default:
		return schema.FieldTypeString, ""
	}
}

// isMap reports whether an object schema is an open-ended map[string]T: it
// declares additionalProperties and no fixed properties.
func isMap(s *spec.Schema) bool {
	return s.AdditionalProperties != nil && len(s.Properties) == 0
}

// refName returns the definitions-relative name a $ref points to, e.g.
// "#/definitions/io.k8s.api.core.v1.PodSpec" -> "io.k8s.api.core.v1.PodSpec",
// or "" if s is not a $ref.
func refName(s *spec.Schema) string {
	const prefix = "#/definitions/"
	ref := s.Ref.String()
	if !strings.HasPrefix(ref, prefix) {
		return ""
	}
	return strings.TrimPrefix(ref, prefix)
}
