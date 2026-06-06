package schema

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// SampleInferrer implements SchemaInferrer by fetching one sample object
// via LIST limit=1 and walking its top-level keys.
// It is used as the fallback adapter in CompositeInferrer.
type SampleInferrer struct {
	client    dynamic.Interface
	namespace string
}

// NewSampleInferrer creates a SampleInferrer.
func NewSampleInferrer(client dynamic.Interface, namespace string) *SampleInferrer {
	return &SampleInferrer{client: client, namespace: namespace}
}

// InferFields fetches one sample object for the given GVR and derives the field list.
// Returns nil when the resource is empty or the client is nil.
func (s *SampleInferrer) InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]Field, error) {
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

	return InferFields(obj), nil
}
