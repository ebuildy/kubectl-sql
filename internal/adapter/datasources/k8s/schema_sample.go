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

// sampleLimit caps how many objects are sampled for schema inference. Sampling a
// small batch (rather than a single object) lets dynamic map keys such as
// metadata.labels.app surface even when they appear on only some objects.
const sampleLimit = 50

func (s *sampleInferrer) Provide(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
	if s.client == nil {
		return nil, nil
	}

	ri := s.client.Resource(gvr)
	opts := metav1.ListOptions{Limit: sampleLimit}

	var items []map[string]interface{}
	if s.namespace != "" {
		list, err := ri.Namespace(s.namespace).List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("schema: sample LIST %s: %w", gvr.Resource, err)
		}
		for i := range list.Items {
			items = append(items, list.Items[i].Object)
		}
	} else {
		list, err := ri.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("schema: sample LIST %s: %w", gvr.Resource, err)
		}
		for i := range list.Items {
			items = append(items, list.Items[i].Object)
		}
	}

	if len(items) == 0 {
		return nil, nil
	}

	// Union the fields inferred from each sampled object so dynamic keys present on
	// only some objects (e.g. metadata.labels.app) are still discovered.
	root := &schema.Field{Name: "root", Type: schema.FieldTypeObject}
	for _, obj := range items {
		fields := schema.InferFields(obj)
		if len(fields) == 0 {
			continue
		}
		if err := mergeSchemas(root, fields); err != nil {
			return nil, fmt.Errorf("schema: sample merge %s: %w", gvr.Resource, err)
		}
	}

	return root.SubFields, nil
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
