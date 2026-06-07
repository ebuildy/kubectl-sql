package k8s

import (
	"context"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
)

// defaultSchemaInferrer provide a hardcoded schema for well-known resources, to avoid expensive OpenAPI/sample inference for common queries like "SELECT * FROM pods".
type defaultSchemaProvider struct {
}

func newDefaultSchemaProvider() *defaultSchemaProvider {
	return &defaultSchemaProvider{}
}

func (c *defaultSchemaProvider) Provide(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
	return []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "namespace", Type: schema.FieldTypeString},
		{Name: "labels", Type: schema.FieldTypeObject},
		{Name: "annotations", Type: schema.FieldTypeObject},
		{Name: "metadata", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "namespace", Type: schema.FieldTypeString},
			{Name: "labels", Type: schema.FieldTypeObject},
			{Name: "annotations", Type: schema.FieldTypeObject},
		}},
		{Name: "spec", Type: schema.FieldTypeObject},
		{Name: "status", Type: schema.FieldTypeObject},
	}, nil
}
