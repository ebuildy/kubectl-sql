package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

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

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
