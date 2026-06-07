package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// schemaInferrer is the internal port for schema inference by GVR. The concrete
// inferrers (OpenAPI, sample, composite) implement it.
type schemaInferrer interface {
	InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error)
}

// --- Composite ---------------------------------------------------------------

// compositeInferrer tries primary then falls back to secondary, merging SubFields.
type compositeInferrer struct {
	primary   schemaInferrer
	secondary schemaInferrer
}

func newCompositeInferrer(primary, secondary schemaInferrer) *compositeInferrer {
	return &compositeInferrer{primary: primary, secondary: secondary}
}

func (c *compositeInferrer) InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
	log := logger.FromContext(ctx)
	log.Debug("inferring schema", logger.String("gvr", gvr.String()))
	start := time.Now()
	defer func() {
		log.Debug("schema inferred",
			logger.String("gvr", gvr.String()),
			logger.Duration("elapsed", time.Since(start)))
	}()

	primary, err := c.primary.InferFields(ctx, gvr)
	if err != nil || len(primary) == 0 {
		log.Debug("schema source: sample inferrer (primary empty)", logger.String("gvr", gvr.String()))
		return c.secondary.InferFields(ctx, gvr)
	}

	secondary, _ := c.secondary.InferFields(ctx, gvr)
	if len(secondary) == 0 {
		log.Debug("schema source: openapi inferrer", logger.String("gvr", gvr.String()))
		return primary, nil
	}
	log.Debug("schema source: openapi inferrer merged with sample subfields", logger.String("gvr", gvr.String()))

	secMap := make(map[string]schema.Field, len(secondary))
	for _, f := range secondary {
		secMap[f.Name] = f
	}

	primarySeen := make(map[string]bool, len(primary))
	for _, f := range primary {
		primarySeen[f.Name] = true
	}

	merged := make([]schema.Field, len(primary))
	copy(merged, primary)
	for i, f := range merged {
		sf, ok := secMap[f.Name]
		if !ok {
			continue
		}
		if f.Type != schema.FieldTypeObject && sf.Type == schema.FieldTypeObject && len(sf.SubFields) > 0 {
			merged[i].Type = sf.Type
			merged[i].SubFields = sf.SubFields
			continue
		}
		if f.Type == schema.FieldTypeObject && len(f.SubFields) == 0 && len(sf.SubFields) > 0 {
			merged[i].SubFields = sf.SubFields
		}
	}

	for _, sf := range secondary {
		if !primarySeen[sf.Name] {
			merged = append(merged, sf)
		}
	}

	return merged, nil
}

// --- Sample ------------------------------------------------------------------

// sampleInferrer infers fields by fetching one sample object (LIST limit=1).
type sampleInferrer struct {
	client    dynamic.Interface
	namespace string
}

func newSampleInferrer(client dynamic.Interface, namespace string) *sampleInferrer {
	return &sampleInferrer{client: client, namespace: namespace}
}

func (s *sampleInferrer) InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
	if s.client == nil {
		return nil, nil
	}

	ri := s.client.Resource(gvr)
	opts := metav1.ListOptions{Limit: 1}

	var obj map[string]interface{}
	if s.namespace != "" {
		list, err := ri.Namespace(s.namespace).List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("schema: sample LIST %s: %w", gvr.Resource, err)
		}
		if len(list.Items) > 0 {
			obj = list.Items[0].Object
		}
	} else {
		list, err := ri.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("schema: sample LIST %s: %w", gvr.Resource, err)
		}
		if len(list.Items) > 0 {
			obj = list.Items[0].Object
		}
	}

	return schema.InferFields(obj), nil
}

// --- OpenAPI -----------------------------------------------------------------

// openAPIInferrer infers fields from the cluster's OpenAPI v3 document.
type openAPIInferrer struct {
	discovery discovery.DiscoveryInterface
}

func newOpenAPIInferrer(disco discovery.DiscoveryInterface) *openAPIInferrer {
	return &openAPIInferrer{discovery: disco}
}

func (o *openAPIInferrer) InferFields(_ context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
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

	if ft == schema.FieldTypeObject && len(s.Properties) > 0 {
		keys := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			if schema.IsIgnoredField(k) {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		f.SubFields = make([]schema.Field, 0, len(keys))
		for _, k := range keys {
			sub := s.Properties[k]
			f.SubFields = append(f.SubFields, schema.Field{Name: k, Type: openAPITypeToFieldType(&sub)})
		}
	}

	return f
}

func openAPITypeToFieldType(s *spec.Schema) schema.FieldType {
	if s == nil || len(s.Type) == 0 {
		return schema.FieldTypeString
	}
	switch s.Type[0] {
	case "boolean":
		return schema.FieldTypeBool
	case "integer":
		return schema.FieldTypeInt
	case "number":
		return schema.FieldTypeFloat
	case "object":
		return schema.FieldTypeObject
	case "array":
		return schema.FieldTypeString
	default:
		return schema.FieldTypeString
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
