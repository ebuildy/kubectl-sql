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

// schemaToField converts a single property schema to a field. For object-typed
// properties it recurses into the referenced (or inline) definition. For
// array-typed properties whose element ($ref'd or inline) is an object, it
// resolves the element's fields into the list field's SubFields (the element
// schema), so list[index]->field works. Scalar/map/unresolvable elements leave
// SubFields nil.
func schemaToField(defs spec.Definitions, name string, s *spec.Schema, depth int, visiting map[string]bool) schema.Field {
	ft, ref := classify(s)
	f := schema.Field{Name: name, Type: ft}

	if depth >= maxDepth {
		return f
	}

	switch ft {
	case schema.FieldTypeObject:
		f.SubFields = objectSubFields(defs, s, ref, depth, visiting)
	case schema.FieldTypeList:
		if item := itemsSchema(s); item != nil {
			if eft, eref := classify(item); eft == schema.FieldTypeObject {
				f.SubFields = objectSubFields(defs, item, eref, depth, visiting)
			}
		}
	}
	return f
}

// objectSubFields resolves the subfields of an object-typed schema, either by
// following its $ref (ref != "") under the cycle guard, or by recursing into its
// inline properties. Returns nil on a cycle or an unresolvable ref (truncating to
// a childless object). Respects the maxDepth cap via the depth+1 recursion.
func objectSubFields(defs spec.Definitions, s *spec.Schema, ref string, depth int, visiting map[string]bool) []schema.Field {
	if ref != "" {
		if visiting[ref] {
			return nil // cycle: truncate to a childless object
		}
		target, ok := defs[ref]
		if !ok {
			return nil
		}
		visiting[ref] = true
		sub := propertiesToFields(defs, target, depth+1, visiting)
		delete(visiting, ref)
		return sub
	}
	// Inline object schema (no $ref): recurse into its own properties.
	return propertiesToFields(defs, *s, depth+1, visiting)
}

// itemsSchema returns the single element schema of an array property, or nil
// when the array carries no resolvable single item schema (nil Items /
// Items.Schema, e.g. a tuple-style schema).
func itemsSchema(s *spec.Schema) *spec.Schema {
	if s == nil || s.Items == nil {
		return nil
	}
	return s.Items.Schema
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
