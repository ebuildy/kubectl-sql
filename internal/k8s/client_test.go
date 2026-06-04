// Package k8s_test tests the k8s client bootstrap.
package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDynamicClient_InvalidKubeconfig(t *testing.T) {
	_, _, err := NewDynamicClient("/nonexistent/kubeconfig", "")
	require.Error(t, err)
}
