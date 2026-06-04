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
}
