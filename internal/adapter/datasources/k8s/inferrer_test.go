package k8s

import (
	"context"
	"testing"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// stubInferrer is a test double for schemaInferrer.
type stubInferrer struct {
	fields []schema.Field
	called bool
}

func (s *stubInferrer) InferFields(_ context.Context, _ k8sschema.GroupVersionResource) ([]schema.Field, error) {
	s.called = true
	return s.fields, nil
}

func TestCompositeInferrer_UsesPrimaryWhenAvailable(t *testing.T) {
	primary := &stubInferrer{fields: []schema.Field{{Name: "name", Type: schema.FieldTypeString}}}
	secondary := &stubInferrer{}
	c := newCompositeInferrer(primary, secondary)

	fields, err := c.InferFields(context.Background(), k8sschema.GroupVersionResource{Resource: "pods"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) == 0 {
		t.Error("expected fields from primary")
	}
	if !secondary.called {
		t.Error("secondary should be called for SubField merging")
	}
}

func TestCompositeInferrer_FallsBackOnEmpty(t *testing.T) {
	primary := &stubInferrer{fields: nil}
	secondary := &stubInferrer{fields: []schema.Field{{Name: "name", Type: schema.FieldTypeString}}}
	c := newCompositeInferrer(primary, secondary)

	fields, err := c.InferFields(context.Background(), k8sschema.GroupVersionResource{Resource: "pods"})
	if err != nil {
		t.Fatal(err)
	}
	if !secondary.called {
		t.Error("secondary should be called when primary returns nil")
	}
	if len(fields) == 0 {
		t.Error("expected fields from secondary")
	}
}
