package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// openAPIInferrer infers fields from the cluster's OpenAPI v3 document.
type openAPIInferrer struct {
	discovery discovery.DiscoveryInterface
}

func newOpenAPIInferrer(disco discovery.DiscoveryInterface) *openAPIInferrer {
	return &openAPIInferrer{discovery: disco}
}

func (o *openAPIInferrer) Provide(_ context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
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

	result := schema.GuaranteedFields()
	return append(result, fields...), nil
}

func (o *openAPIInferrer) fetchSchema(gvr k8sschema.GroupVersionResource) (*spec.Schema, error) {
	openapiClient := o.discovery.OpenAPIV3()
	paths, err := openapiClient.Paths()
	if err != nil {
		return nil, fmt.Errorf("openapi: fetch paths: %w", err)
	}

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

	resourceKind := gvr.Resource
	for key, s := range doc.Components.Schemas {
		if schemaKeyMatchesResource(key, resourceKind) {
			schemaCopy := *s
			return &schemaCopy, nil
		}
	}

	return nil, nil
}

func schemaKeyMatchesResource(key, resource string) bool {
	last := key
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '.' {
			last = key[i+1:]
			break
		}
	}
	kind := lower(last)
	return kind+"s" == resource || kind == resource
}

func schemaToFields(s *spec.Schema) []schema.Field {
	if s == nil || len(s.Properties) == 0 {
		return nil
	}

	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		if schema.IsGuaranteedName(k) || schema.IsIgnoredField(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fields := make([]schema.Field, 0, len(keys))
	for _, k := range keys {
		prop := s.Properties[k]
		fields = append(fields, openAPISchemaToField(k, &prop))
	}
	return fields
}

func openAPISchemaToField(name string, s *spec.Schema) schema.Field {
	ft := openAPITypeToFieldType(s)
	f := schema.Field{Name: name, Type: ft}

	switch ft {
	case schema.FieldTypeObject:
		f.SubFields = openAPIChildFields(s)
	case schema.FieldTypeList:
		// Resolve the array element's object schema into the list field's SubFields
		// (the element schema), mirroring the swagger generator. Scalar/map/
		// unresolvable elements leave SubFields nil.
		if item := openAPIItemsSchema(s); item != nil && openAPITypeToFieldType(item) == schema.FieldTypeObject {
			f.SubFields = openAPIChildFields(item)
		}
	}

	return f
}

// openAPIChildFields builds the (one-level) subfields of an object schema from
// its inline properties, sorted and with server-managed fields dropped. Returns
// nil when the schema has no inline properties.
func openAPIChildFields(s *spec.Schema) []schema.Field {
	if len(s.Properties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		if schema.IsIgnoredField(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]schema.Field, 0, len(keys))
	for _, k := range keys {
		sub := s.Properties[k]
		out = append(out, schema.Field{Name: k, Type: openAPITypeToFieldType(&sub)})
	}
	return out
}

// openAPIItemsSchema returns the single element schema of an array property, or
// nil when the array carries no resolvable single item schema.
func openAPIItemsSchema(s *spec.Schema) *spec.Schema {
	if s == nil || s.Items == nil {
		return nil
	}
	return s.Items.Schema
}

// isOpenAPIMap reports whether an object schema is an open-ended map[string]T:
// it declares additionalProperties (a value schema) and no fixed properties.
func isOpenAPIMap(s *spec.Schema) bool {
	return s.AdditionalProperties != nil && len(s.Properties) == 0
}

func openAPITypeToFieldType(s *spec.Schema) schema.FieldType {
	if s == nil {
		return schema.FieldTypeString
	}
	if len(s.Type) == 0 {
		// Structural schemas (e.g. metadata -> $ref ObjectMeta) carry no explicit
		// "type" but are objects. Detect them via $ref, nested properties, a map
		// value (additionalProperties), or composition keywords.
		switch {
		case isOpenAPIMap(s):
			return schema.FieldTypeMap
		case s.Ref.String() != "" || len(s.Properties) > 0 ||
			len(s.AllOf) > 0 || len(s.OneOf) > 0 || len(s.AnyOf) > 0:
			return schema.FieldTypeObject
		default:
			return schema.FieldTypeString
		}
	}
	switch s.Type[0] {
	case "boolean":
		return schema.FieldTypeBool
	case "integer":
		return schema.FieldTypeInt
	case "number":
		return schema.FieldTypeFloat
	case "object":
		if isOpenAPIMap(s) {
			return schema.FieldTypeMap
		}
		return schema.FieldTypeObject
	case "array":
		return schema.FieldTypeList
	default:
		return schema.FieldTypeString
	}
}
