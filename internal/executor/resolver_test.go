// Package executor_test tests field resolution on unstructured objects.
package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveField(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "my-pod",
			"namespace": "default",
			"labels": map[string]interface{}{
				"app":  "nginx",
				"tier": "frontend",
			},
		},
		"status": map[string]interface{}{
			"phase": "Running",
		},
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"name": "config",
					"configMap": map[string]interface{}{
						"name": "nginx-config",
					},
				},
				map[string]interface{}{
					"name":     "data",
					"emptyDir": map[string]interface{}{},
				},
			},
		},
		"count": int64(3),
	}

	t.Run("top-level field", func(t *testing.T) {
		assert.Equal(t, int64(3), ResolveField(obj, "count"))
	})

	t.Run("nested field", func(t *testing.T) {
		assert.Equal(t, "Running", ResolveField(obj, "status.phase"))
	})

	t.Run("deeply nested field", func(t *testing.T) {
		assert.Equal(t, "my-pod", ResolveField(obj, "metadata.name"))
	})

	t.Run("missing top-level field returns nil", func(t *testing.T) {
		assert.Nil(t, ResolveField(obj, "doesnotexist"))
	})

	t.Run("missing nested field returns nil", func(t *testing.T) {
		assert.Nil(t, ResolveField(obj, "status.reason"))
	})

	t.Run("bracket label access", func(t *testing.T) {
		assert.Equal(t, "nginx", ResolveField(obj, "metadata.labels['app']"))
	})

	t.Run("bracket label access with leading dot", func(t *testing.T) {
		assert.Equal(t, "frontend", ResolveField(obj, ".metadata.labels['tier']"))
	})

	t.Run("bracket label missing key returns nil", func(t *testing.T) {
		assert.Nil(t, ResolveField(obj, "metadata.labels['missing']"))
	})

	t.Run("numeric array index", func(t *testing.T) {
		assert.Equal(t, "config", ResolveField(obj, "spec.volumes[0].name"))
	})

	t.Run("numeric array index second element", func(t *testing.T) {
		assert.Equal(t, "data", ResolveField(obj, "spec.volumes[1].name"))
	})

	t.Run("numeric array index nested field", func(t *testing.T) {
		assert.Equal(t, "nginx-config", ResolveField(obj, "spec.volumes[0].configMap.name"))
	})

	t.Run("numeric array index out of bounds returns nil", func(t *testing.T) {
		assert.Nil(t, ResolveField(obj, "spec.volumes[5].name"))
	})

	t.Run("numeric array index on non-slice returns nil", func(t *testing.T) {
		assert.Nil(t, ResolveField(obj, "metadata[0]"))
	})
}
