// Package k8s bootstraps the Kubernetes dynamic client from kubeconfig.
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

// NewDynamicClient builds a dynamic Kubernetes client, REST mapper, and discovery client
// from the given kubeconfig path and context name. Empty strings use the default kubeconfig and context.
func NewDynamicClient(kubeconfig, kubeContext string) (dynamic.Interface, meta.RESTMapper, discovery.DiscoveryInterface, error) {
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
