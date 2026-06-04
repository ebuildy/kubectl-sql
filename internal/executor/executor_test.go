// Package executor_test tests the Kubernetes datasource.
package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fakeRESTMapper implements meta.RESTMapper with a fixed mapping table.
type fakeRESTMapper struct {
	mapping map[string]schema.GroupVersionResource
}

func (f *fakeRESTMapper) ResourceFor(partial schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	if gvr, ok := f.mapping[partial.Resource]; ok {
		return gvr, nil
	}
	return schema.GroupVersionResource{}, &meta.NoResourceMatchError{PartialResource: partial}
}

// Remaining methods are no-ops to satisfy the interface.
func (f *fakeRESTMapper) KindFor(_ schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (f *fakeRESTMapper) KindsFor(_ schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, nil
}
func (f *fakeRESTMapper) ResourcesFor(_ schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, nil
}
func (f *fakeRESTMapper) RESTMapping(_ schema.GroupKind, _ ...string) (*meta.RESTMapping, error) {
	return nil, nil
}
func (f *fakeRESTMapper) RESTMappings(_ schema.GroupKind, _ ...string) ([]*meta.RESTMapping, error) {
	return nil, nil
}
func (f *fakeRESTMapper) ResourceSingularizer(_ string) (string, error) { return "", nil }

func newFakeMapper() meta.RESTMapper {
	return &fakeRESTMapper{
		mapping: map[string]schema.GroupVersionResource{
			"pods":        {Group: "", Version: "v1", Resource: "pods"},
			"pod":         {Group: "", Version: "v1", Resource: "pods"},
			"deployments": {Group: "apps", Version: "v1", Resource: "deployments"},
		},
	}
}

func TestGetTable_PluralResolvesGVR(t *testing.T) {
	db := NewKubernetesDatabase(nil, newFakeMapper(), "", 500)
	impl, sch, err := db.GetTable(context.Background(), "pods", nil)
	require.NoError(t, err)
	assert.NotNil(t, impl)
	assert.NotEmpty(t, sch.Fields)
}

func TestGetTable_SingularResolvesGVR(t *testing.T) {
	db := NewKubernetesDatabase(nil, newFakeMapper(), "", 500)
	impl, _, err := db.GetTable(context.Background(), "pod", nil)
	require.NoError(t, err)
	assert.NotNil(t, impl)
}

func TestGetTable_UnknownReturnsError(t *testing.T) {
	db := NewKubernetesDatabase(nil, newFakeMapper(), "", 500)
	_, _, err := db.GetTable(context.Background(), "doesnotexist", nil)
	require.Error(t, err)
}
