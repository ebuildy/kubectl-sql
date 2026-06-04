//go:build integration

package integration

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	namespacesGVR  = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	podsGVR        = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	deploymentsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	configmapsGVR  = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
)

// SeedFixtures creates a "main" namespace with exactly 2 pods, then 10 random
// namespaces each containing 3-5 Pods (phase=Running), 1-2 Deployments, and
// 1-2 ConfigMaps. Returns the list of random namespace names (excludes "main").
func SeedFixtures(ctx context.Context, dynClient dynamic.Interface) ([]string, error) {
	if err := seedNamespace(ctx, dynClient, "main", 2, 0, 0); err != nil {
		return nil, err
	}

	const namespaceCount = 10
	namespaces := make([]string, 0, namespaceCount)

	for i := 0; i < namespaceCount; i++ {
		nsName := randomName()
		podCount := 3 + rng.Intn(3)   // 3-5
		deployCount := 1 + rng.Intn(2) // 1-2
		cmCount := 1 + rng.Intn(2)     // 1-2
		if err := seedNamespace(ctx, dynClient, nsName, podCount, deployCount, cmCount); err != nil {
			return nil, err
		}
		namespaces = append(namespaces, nsName)
	}

	return namespaces, nil
}

func seedNamespace(ctx context.Context, dynClient dynamic.Interface, nsName string, pods, deployments, configmaps int) error {
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]interface{}{"name": nsName},
		},
	}
	if _, err := dynClient.Resource(namespacesGVR).Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create namespace %s: %w", nsName, err)
	}
	for p := 0; p < pods; p++ {
		if err := createPod(ctx, dynClient, nsName); err != nil {
			return err
		}
	}
	for d := 0; d < deployments; d++ {
		if err := createDeployment(ctx, dynClient, nsName); err != nil {
			return err
		}
	}
	for c := 0; c < configmaps; c++ {
		if err := createConfigMap(ctx, dynClient, nsName); err != nil {
			return err
		}
	}
	return nil
}

func createPod(ctx context.Context, dynClient dynamic.Interface, ns string) error {
	name := randomName()
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]interface{}{"name": name, "namespace": ns},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": "nginx:latest",
					},
				},
			},
		},
	}
	created, err := dynClient.Resource(podsGVR).Namespace(ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create pod %s/%s: %w", ns, name, err)
	}

	// PATCH status subresource to set phase=Running (envtest has no scheduler)
	statusPatch := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":            created.GetName(),
				"namespace":       ns,
				"resourceVersion": created.GetResourceVersion(),
			},
			"status": map[string]interface{}{
				"phase": string(corev1.PodRunning),
			},
		},
	}
	_, err = dynClient.Resource(podsGVR).Namespace(ns).UpdateStatus(ctx, statusPatch, metav1.UpdateOptions{})
	return err
}

func createDeployment(ctx context.Context, dynClient dynamic.Interface, ns string) error {
	name := randomName()
	deploy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": name, "namespace": ns},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{"app": name},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": name},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "nginx:latest",
							},
						},
					},
				},
			},
		},
	}
	_, err := dynClient.Resource(deploymentsGVR).Namespace(ns).Create(ctx, deploy, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create deployment %s/%s: %w", ns, name, err)
	}
	return nil
}

func createConfigMap(ctx context.Context, dynClient dynamic.Interface, ns string) error {
	name := randomName()
	cm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": name, "namespace": ns},
			"data":       map[string]interface{}{"key": "value"},
		},
	}
	_, err := dynClient.Resource(configmapsGVR).Namespace(ns).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create configmap %s/%s: %w", ns, name, err)
	}
	return nil
}
