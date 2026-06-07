package k8s

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
	InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error)
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

func (c *strategicSchemaProvider) InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
	log := logger.FromContext(ctx)
	log.Debug("inferring schema", logger.String("gvr", gvr.String()))
	start := time.Now()
	defer func() {
		log.Debug("schema inferred",
			logger.String("gvr", gvr.String()),
			logger.Duration("elapsed", time.Since(start)))
	}()

	defaultProvider := newDefaultSchemaProvider()

	fields, _ := defaultProvider.InferFields(ctx, gvr)

	root := &schema.Field{Name: "root", Type: schema.FieldTypeObject, SubFields: fields}

	openAPIProvider := newOpenAPIInferrer(c.disco)
	// sampleProvider := newSampleInferrer(c.dyn, c.namespace)

	primary, err := openAPIProvider.InferFields(ctx, gvr)
	if err != nil || len(primary) == 0 {
		log.Debug("schema source: sample inferrer (primary empty)", logger.String("gvr", gvr.String()))
	}

	if len(primary) > 0 {
		err := mergeSchemas(root, primary)
		if err != nil {
			log.Error("schema source: openapi inferrer (primary error)", logger.String("gvr", gvr.String()))
			return root.SubFields, nil
		}
	} else {
		sampleProvider := newSampleInferrer(c.dyn, c.namespace)
		secondary, err := sampleProvider.InferFields(ctx, gvr)
		if err != nil {
			log.Error("schema source: openapi inferrer (secondary error)", logger.String("gvr", gvr.String()))
			return primary, nil
		}
		if len(secondary) == 0 {
			log.Debug("schema source: openapi inferrer (secondary empty)", logger.String("gvr", gvr.String()))
			return primary, nil
		}
		log.Debug("schema source: openapi inferrer merged with sample subfields", logger.String("gvr", gvr.String()))

	}

	return root.SubFields, nil

	// secondary, _ := c.secondary.InferFields(ctx, gvr)
	// if len(secondary) == 0 {
	// 	log.Debug("schema source: openapi inferrer", logger.String("gvr", gvr.String()))
	// 	return primary, nil
	// }
	// log.Debug("schema source: openapi inferrer merged with sample subfields", logger.String("gvr", gvr.String()))

	// primarySeen := make(map[string]bool, len(primary))
	// for _, f := range primary {
	// 	primarySeen[f.Name] = true
	// }

	// merged := make([]schema.Field, len(primary))
	// copy(merged, primary)
	// for i, f := range merged {
	// 	sf, ok := secMap[f.Name]
	// 	if !ok {
	// 		continue
	// 	}
	// 	if f.Type != schema.FieldTypeObject && sf.Type == schema.FieldTypeObject && len(sf.SubFields) > 0 {
	// 		merged[i].Type = sf.Type
	// 		merged[i].SubFields = sf.SubFields
	// 		continue
	// 	}
	// 	if f.Type == schema.FieldTypeObject && len(f.SubFields) == 0 && len(sf.SubFields) > 0 {
	// 		merged[i].SubFields = sf.SubFields
	// 	}
	// 	log.Debug("field", logger.String("name", f.Name), logger.String("path", f.Path), logger.String("type", string(f.Type)))
	// }

	// for _, sf := range secondary {
	// 	if !primarySeen[sf.Name] {
	// 		merged = append(merged, sf)
	// 	}
	// }

	// return merged, nil
}

// mergeSchemas merges two field lists, preferring dest but filling in missing SubFields from source when possible.
func mergeSchemas(root *schema.Field, fields []schema.Field) error {
	fieldsMap := make(map[string]*schema.Field, len(root.SubFields))
	for i := range root.SubFields {
		fieldsMap[root.SubFields[i].Name] = &root.SubFields[i]
	}

	for _, f := range fields {
		sf, ok := fieldsMap[f.Name]
		if ok {
			if f.Type != sf.Type {
				return fmt.Errorf("field type mismatch for field '%s': dest type '%s', source type '%s'", f.Name, sf.Type, f.Type)
			}
			if sf.Type == schema.FieldTypeObject && len(f.SubFields) > 0 {
				err := mergeSchemas(sf, f.SubFields)
				if err != nil {
					return err
				}
			}
		} else {
			root.SubFields = append(root.SubFields, f)
		}
	}
	return nil
}
