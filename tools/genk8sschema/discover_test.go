package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

func loadFixture(t *testing.T) *spec.Swagger {
	t.Helper()
	data, err := os.ReadFile("../../internal/adapter/datasources/k8s/testdata/swagger_fixture.json")
	require.NoError(t, err)

	doc, err := parseSwagger(data)
	require.NoError(t, err)
	return doc
}

func TestDiscoverResources(t *testing.T) {
	doc := loadFixture(t)

	resources, err := discoverResources(doc)
	require.NoError(t, err)
	require.Len(t, resources, 2, "Orphan (no GVK) and the non-list pods/{name}/status path must be excluded")

	assert.Equal(t, resource{group: "", version: "v1", name: "pods", defName: "test.Pod"}, resources[0])
	assert.Equal(t, resource{group: "example.com", version: "v1", name: "widgets", defName: "test.Widget"}, resources[1])

	assert.Equal(t, "/v1/pods", resources[0].key())
	assert.Equal(t, "example.com/v1/widgets", resources[1].key())
}

func TestDiscoverResourcesDeterministicOrder(t *testing.T) {
	doc := loadFixture(t)

	first, err := discoverResources(doc)
	require.NoError(t, err)

	second, err := discoverResources(doc)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}
