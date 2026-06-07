// Package k8s is the client-go adapter for the Kubernetes data-source port
// (internal/port/datasources/k8s). It is the ONLY package (besides the cmd
// composition root) that imports k8s.io/client-go, k8s.io/apimachinery,
// k8s.io/kube-openapi, or k8s.io/client-go/discovery. All client-go types are
// mapped to domain types at this boundary.
package k8s

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// newDynamicClient builds a dynamic client, REST mapper, and discovery client
// from the given kubeconfig path and context name. Empty strings use defaults.
func newDynamicClient(kubeconfig, kubeContext string) (dynamic.Interface, meta.RESTMapper, discovery.DiscoveryInterface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		overrides,
	).ClientConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("k8s: build config: %w", err)
	}
	cfg.Timeout = 3 * time.Second

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("k8s: dynamic client: %w", err)
	}

	discoClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("k8s: discovery client: %w", err)
	}

	groupResources, err := restmapper.GetAPIGroupResources(discoClient)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("k8s: API group resources: %w", err)
	}

	mapper := restmapper.NewShortcutExpander(
		restmapper.NewDiscoveryRESTMapper(groupResources),
		discoClient,
		nil,
	)

	return dynClient, mapper, discoClient, nil
}
