package schema

import (
	"context"
	"time"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ebuildy/kubectl-sql/internal/port/logger"
)

// CompositeInferrer tries the primary inferrer and falls back to the secondary
// when the primary returns nil or an empty field list.
// When both return results, SubFields are merged: for any FieldTypeObject field
// the primary returned without SubFields (e.g. unresolved $ref in OpenAPI), the
// secondary's SubFields for that field are adopted.
type CompositeInferrer struct {
	primary   SchemaInferrer
	secondary SchemaInferrer
}

// NewCompositeInferrer creates a CompositeInferrer.
func NewCompositeInferrer(primary, secondary SchemaInferrer) *CompositeInferrer {
	return &CompositeInferrer{primary: primary, secondary: secondary}
}

// InferFields calls the primary inferrer. If it returns nil or empty, falls back to secondary.
// Otherwise merges secondary SubFields into primary object fields that have none.
func (c *CompositeInferrer) InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]Field, error) {
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

	// Fetch secondary to fill SubFields for ref-only object fields.
	secondary, _ := c.secondary.InferFields(ctx, gvr)
	if len(secondary) == 0 {
		log.Debug("schema source: openapi inferrer", logger.String("gvr", gvr.String()))
		return primary, nil
	}
	log.Debug("schema source: openapi inferrer merged with sample subfields", logger.String("gvr", gvr.String()))

	secMap := make(map[string]Field, len(secondary))
	for _, f := range secondary {
		secMap[f.Name] = f
	}

	// Build index of primary field names.
	primarySeen := make(map[string]bool, len(primary))
	for _, f := range primary {
		primarySeen[f.Name] = true
	}

	merged := make([]Field, len(primary))
	copy(merged, primary)
	for i, f := range merged {
		sf, ok := secMap[f.Name]
		if !ok {
			continue
		}
		// Primary field typed as String (unresolved $ref) but secondary knows it's an
		// object with SubFields — adopt secondary's type and SubFields.
		if f.Type != FieldTypeObject && sf.Type == FieldTypeObject && len(sf.SubFields) > 0 {
			merged[i].Type = sf.Type
			merged[i].SubFields = sf.SubFields
			continue
		}
		// Primary already typed it as Object but has no SubFields — fill them in.
		if f.Type == FieldTypeObject && len(f.SubFields) == 0 && len(sf.SubFields) > 0 {
			merged[i].SubFields = sf.SubFields
		}
	}

	// Append secondary-only fields (e.g. flattened slice index columns like
	// spec_volumes_0_configMap that OpenAPI doesn't know about).
	for _, sf := range secondary {
		if !primarySeen[sf.Name] {
			merged = append(merged, sf)
		}
	}

	return merged, nil
}
