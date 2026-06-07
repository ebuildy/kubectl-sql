package k8s

// This file contains unit tests for the strategic schema provider and its merging logic.
// The strategic provider is the main entry point for schema inference by GVR, and it orchestrates multiple underlying inferrers (default, OpenAPI, sample) and merges their results.
// These tests validate the merging logic in isolation, while the envtest-based integration test validates the end-to-end behavior of all layers working together.

import (
	"context"
	"fmt"
	"time"

	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

// schemaInferrer is the internal port for schema inference by GVR. The concrete
// inferrers (OpenAPI, sample, composite) implement it.
type schemaInferrer interface {
	Provide(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error)
}

// --- Composite ---------------------------------------------------------------

// strategicSchemaProvider tries primary then falls back to secondary, merging SubFields.
type strategicSchemaProvider struct {
	namespace string
	disco     discovery.DiscoveryInterface
	dyn       dynamic.Interface
}

func newStrategicSchemaProvider(namespace string, disco discovery.DiscoveryInterface, dyn dynamic.Interface) *strategicSchemaProvider {
	return &strategicSchemaProvider{
		namespace: namespace,
		disco:     disco,
		dyn:       dyn,
	}
}

func (c *strategicSchemaProvider) Provide(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
	log := logger.FromContext(ctx)
	log.Debug("inferring schema", logger.String("gvr", gvr.String()))
	start := time.Now()
	defer func() {
		log.Debug("schema inferred",
			logger.String("gvr", gvr.String()),
			logger.Duration("elapsed", time.Since(start)))
	}()

	// Start from the hardcoded default baseline (name, namespace, metadata, spec, status, …).
	defaultProvider := newDefaultSchemaProvider()
	fields, _ := defaultProvider.Provide(ctx, gvr)
	root := &schema.Field{Name: "root", Type: schema.FieldTypeObject, SubFields: fields}

	// Layer 1: enrich with OpenAPI v3 fields (structural depth for spec/status/…).
	openAPIProvider := newOpenAPIInferrer(c.disco)
	if openapiFields, err := openAPIProvider.Provide(ctx, gvr); err != nil {
		log.Debug("schema source: openapi inferrer error", logger.String("gvr", gvr.String()), logger.String("err", err.Error()))
	} else if len(openapiFields) > 0 {
		if err := mergeSchemas(root, openapiFields); err != nil {
			log.Error("schema source: openapi merge error", logger.String("gvr", gvr.String()), logger.String("err", err.Error()))
		} else {
			log.Debug("schema source: openapi merged", logger.String("gvr", gvr.String()))
		}
	}

	// Layer 2: enrich with a sample object (dynamic depth, e.g. metadata->labels->app).
	sampleProvider := newSampleInferrer(c.dyn, c.namespace)
	if sampleFields, err := sampleProvider.Provide(ctx, gvr); err != nil {
		log.Debug("schema source: sample inferrer error", logger.String("gvr", gvr.String()), logger.String("err", err.Error()))
	} else if len(sampleFields) > 0 {
		if err := mergeSchemas(root, sampleFields); err != nil {
			log.Error("schema source: sample merge error", logger.String("gvr", gvr.String()), logger.String("err", err.Error()))
		} else {
			log.Debug("schema source: sample merged", logger.String("gvr", gvr.String()))
		}
	}

	return root.SubFields, nil
}

// mergeSchemas layers a source field list onto the destination tree rooted at root.
// Fields absent from the destination are appended. Matching object fields are merged
// recursively so subfields from either source accumulate. When one side carries
// subfields (an object) and the other is a leaf of a different type, the richer
// object form wins (enrichment). A genuine leaf-vs-leaf type conflict (neither side
// an object) is reported as an error so callers can decide how to proceed.
func mergeSchemas(root *schema.Field, fields []schema.Field) error {
	// Index existing destination fields by name. Index by position rather than by
	// pointer: appending new fields below can reallocate root.SubFields, which would
	// invalidate any &root.SubFields[i] pointers held across iterations.
	indexByName := make(map[string]int, len(root.SubFields))
	for i := range root.SubFields {
		indexByName[root.SubFields[i].Name] = i
	}

	var newFields []schema.Field
	for _, f := range fields {
		idx, ok := indexByName[f.Name]
		if !ok {
			newFields = append(newFields, f)
			continue
		}
		dst := &root.SubFields[idx]

		if f.Type == dst.Type {
			if dst.Type == schema.FieldTypeObject && len(f.SubFields) > 0 {
				if err := mergeSchemas(dst, f.SubFields); err != nil {
					return err
				}
			}
			continue
		}

		// Types differ: prefer the object form so nested access keeps working.
		switch {
		case f.Type == schema.FieldTypeObject:
			dst.Type = schema.FieldTypeObject
			dst.SubFields = nil
			if len(f.SubFields) > 0 {
				if err := mergeSchemas(dst, f.SubFields); err != nil {
					return err
				}
			}
		case dst.Type == schema.FieldTypeObject:
			// Destination already an object; keep it (don't downgrade to a leaf).
		default:
			return fmt.Errorf("field type mismatch for field '%s': dest type '%s', source type '%s'", f.Name, dst.Type, f.Type)
		}
	}

	// Append all new fields once, after matched fields have been merged in place.
	root.SubFields = append(root.SubFields, newFields...)
	return nil
}
