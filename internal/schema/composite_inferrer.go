package schema

import (
	"context"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
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
	primary, err := c.primary.InferFields(ctx, gvr)
	if err != nil || len(primary) == 0 {
		return c.secondary.InferFields(ctx, gvr)
	}

	// Fetch secondary to fill SubFields for ref-only object fields.
	secondary, _ := c.secondary.InferFields(ctx, gvr)
	if len(secondary) == 0 {
		return primary, nil
	}

	secMap := make(map[string]Field, len(secondary))
	for _, f := range secondary {
		secMap[f.Name] = f
	}

	merged := make([]Field, len(primary))
	copy(merged, primary)
	for i, f := range merged {
		sf, ok := secMap[f.Name]
		if !ok {
			continue
		}
		// If the primary field has no SubFields but the secondary knows this is an
		// object with SubFields (e.g. primary returned a $ref-only schema typed as
		// String), adopt the secondary's type and SubFields entirely.
		if f.Type != FieldTypeObject && sf.Type == FieldTypeObject && len(sf.SubFields) > 0 {
			merged[i].Type = sf.Type
			merged[i].SubFields = sf.SubFields
			continue
		}
		// If primary already typed it as Object but has no SubFields, fill them in.
		if f.Type == FieldTypeObject && len(f.SubFields) == 0 && len(sf.SubFields) > 0 {
			merged[i].SubFields = sf.SubFields
		}
	}
	return merged, nil
}
