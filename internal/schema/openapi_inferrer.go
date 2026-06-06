package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// OpenAPIInferrer implements SchemaInferrer by fetching the cluster's OpenAPI v3
// document and deriving the field list from the resource's schema definition.
// It is used as the primary adapter in CompositeInferrer.
type OpenAPIInferrer struct {
	discovery discovery.DiscoveryInterface
}

// NewOpenAPIInferrer creates an OpenAPIInferrer.
func NewOpenAPIInferrer(disco discovery.DiscoveryInterface) *OpenAPIInferrer {
	return &OpenAPIInferrer{discovery: disco}
}

// InferFields fetches the OpenAPI v3 schema for the given GVR and returns its fields.
// Returns nil (not error) when no schema is found — triggers CompositeInferrer fallback.
func (o *OpenAPIInferrer) InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]Field, error) {
	if o.discovery == nil {
		return nil, nil
	}

	openAPISchema, err := o.fetchSchema(gvr)
	if err != nil || openAPISchema == nil {
		return nil, nil //nolint:nilerr // no schema → fall back to sample inferrer
	}

	fields := schemaToFields(openAPISchema)
	if len(fields) == 0 {
		return nil, nil
	}

	result := make([]Field, len(guaranteedFields))
	copy(result, guaranteedFields)
	return append(result, fields...), nil
}

// fetchSchema retrieves the OpenAPI v3 spec for the group/version that contains gvr,
// then locates the schema for the specific resource kind.
func (o *OpenAPIInferrer) fetchSchema(gvr k8sschema.GroupVersionResource) (*spec.Schema, error) {
	openapiClient := o.discovery.OpenAPIV3()
	paths, err := openapiClient.Paths()
	if err != nil {
		return nil, fmt.Errorf("openapi: fetch paths: %w", err)
	}

	// Build the path key: "apis/<group>/<version>" or "api/v1" for core.
	var pathKey string
	if gvr.Group == "" {
		pathKey = fmt.Sprintf("api/%s", gvr.Version)
	} else {
		pathKey = fmt.Sprintf("apis/%s/%s", gvr.Group, gvr.Version)
	}

	gvPath, ok := paths[pathKey]
	if !ok {
		return nil, nil
	}

	schemaBytes, err := gvPath.Schema("application/json")
	if err != nil {
		return nil, fmt.Errorf("openapi: fetch schema for %s: %w", pathKey, err)
	}

	var doc spec3.OpenAPI
	if err := json.Unmarshal(schemaBytes, &doc); err != nil {
		return nil, fmt.Errorf("openapi: parse schema: %w", err)
	}

	if doc.Components == nil || doc.Components.Schemas == nil {
		return nil, nil
	}

	// Locate the resource schema by scanning for a key that ends with the resource kind.
	// OpenAPI keys look like "io.k8s.api.core.v1.Pod" — the kind is the last segment.
	resourceKind := gvr.Resource // plural; we match against the kind below
	for key, s := range doc.Components.Schemas {
		// Match by kind name embedded in the dotted key (last segment, case-insensitive plural check).
		// e.g. "io.k8s.api.core.v1.Pod" for resource "pods"
		if schemaKeyMatchesResource(key, resourceKind) {
			schemaCopy := *s
			return &schemaCopy, nil
		}
	}

	return nil, nil
}

// schemaKeyMatchesResource returns true if the OpenAPI schema key corresponds to the
// given resource name (plural). E.g. key "io.k8s.api.core.v1.Pod" matches resource "pods".
func schemaKeyMatchesResource(key, resource string) bool {
	// Get the last segment of the dotted key (the kind name).
	last := key
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '.' {
			last = key[i+1:]
			break
		}
	}
	// Compare lowercased kind+"s" against resource (simple pluralisation).
	kind := lower(last)
	return kind+"s" == resource || kind == resource
}

// schemaToFields converts an OpenAPI spec.Schema's properties into a []Field.
// Map properties → FieldTypeObject with SubFields; arrays → FieldTypeString; scalars directly.
func schemaToFields(s *spec.Schema) []Field {
	if s == nil || len(s.Properties) == 0 {
		return nil
	}

	// Sort keys for deterministic order.
	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		if guaranteedNames[k] || isIgnoredField(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fields := make([]Field, 0, len(keys))
	for _, k := range keys {
		prop := s.Properties[k]
		fields = append(fields, openAPISchemaToField(k, &prop))
	}
	return fields
}

// openAPISchemaToField converts a single OpenAPI property schema to a Field.
func openAPISchemaToField(name string, s *spec.Schema) Field {
	ft := openAPITypeToFieldType(s)
	f := Field{Name: name, Type: ft}

	if ft == FieldTypeObject && len(s.Properties) > 0 {
		keys := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			if isIgnoredField(k) {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		f.SubFields = make([]Field, 0, len(keys))
		for _, k := range keys {
			sub := s.Properties[k]
			f.SubFields = append(f.SubFields, Field{Name: k, Type: openAPITypeToFieldType(&sub)})
		}
	}

	return f
}

// openAPITypeToFieldType maps an OpenAPI schema type string to FieldType.
func openAPITypeToFieldType(s *spec.Schema) FieldType {
	if s == nil || len(s.Type) == 0 {
		return FieldTypeString
	}
	switch s.Type[0] {
	case "boolean":
		return FieldTypeBool
	case "integer":
		return FieldTypeInt
	case "number":
		return FieldTypeFloat
	case "object":
		return FieldTypeObject
	case "array":
		return FieldTypeString // serialised as JSON
	default:
		return FieldTypeString
	}
}

// lower lowercases a string without importing strings to keep the file lean.
func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
