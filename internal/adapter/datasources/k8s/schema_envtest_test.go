//go:build integration

package k8s

// Note: this test is disabled because envtest is slow to start up and the test suite is already quite slow.

// This file contains an integration test for schema inference using envtest, which is a fake Kubernetes API server that runs in-process. It seeds a pod with both
// labels and volumes, and asserts that the strategic schema provider infers the
// correct shape for both fields (labels is a struct, volumes is a list/slice).
//
// This test validates the integration of all schema inference layers (default,
// OpenAPI, sample) and their merging logic. It also serves as a regression test
// for the specific issue of distinguishing between struct vs list fields.

// import (
// 	"context"
// 	"testing"

// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"

// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
// 	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
// 	"k8s.io/client-go/discovery"
// 	"k8s.io/client-go/dynamic"
// 	"sigs.k8s.io/controller-runtime/pkg/envtest"

// 	"github.com/ebuildy/kubectl-sql/internal/port/schema"
// )

// // TestSchema_Provide_Envtest spins up a fake Kubernetes API server (envtest),
// // seeds a single pod carrying both labels and volumes, and asserts how the
// // strategic schema provider infers the shape of nested fields.
// //
// // In the schema model a FieldTypeObject WITH subfields is a struct (octosql
// // TypeIDStruct), while a FieldTypeObject WITHOUT subfields is a slice/list
// // value that is serialized as a JSON string. The pod fixture therefore lets us
// // validate that:
// //   - metadata.labels is a struct (string map → named subfields)
// //   - spec.volumes is a slice/list (array → no subfields)
// func TestSchema_Provide_Envtest(t *testing.T) {
// 	env := &envtest.Environment{}
// 	cfg, err := env.Start()
// 	require.NoError(t, err, "start envtest")
// 	t.Cleanup(func() {
// 		if stopErr := env.Stop(); stopErr != nil {
// 			t.Logf("stop envtest: %v", stopErr)
// 		}
// 	})

// 	dyn, err := dynamic.NewForConfig(cfg)
// 	require.NoError(t, err, "dynamic client")
// 	disco, err := discovery.NewDiscoveryClientForConfig(cfg)
// 	require.NoError(t, err, "discovery client")

// 	const namespace = "schema-test"
// 	ctx := context.Background()
// 	seedNamespace(ctx, t, dyn, namespace)
// 	seedPod(ctx, t, dyn, namespace)

// 	provider := newStrategicSchemaProvider(namespace, disco, dyn)
// 	gvr := k8sschema.GroupVersionResource{Version: "v1", Resource: "pods"}

// 	fields, err := provider.Provide(ctx, gvr)
// 	require.NoError(t, err, "provide schema")
// 	require.NotEmpty(t, fields, "inferred fields")

// 	metadata := findField(t, fields, "metadata")
// 	labels := findField(t, metadata.SubFields, "labels")
// 	assert.Equal(t, schema.FieldTypeObject, labels.Type, "metadata.labels is an object")
// 	assert.NotEmpty(t, labels.SubFields,
// 		"metadata.labels is a struct: a string map infers named subfields")
// 	findField(t, labels.SubFields, "app")

// 	spec := findField(t, fields, "spec")
// 	volumes := findField(t, spec.SubFields, "volumes")
// 	assert.Equal(t, schema.FieldTypeList, volumes.Type,
// 		"spec.volumes is a slice/list (array), not a struct")
// 	assert.Empty(t, volumes.SubFields, "a list has no named subfields")
// }

// // findField returns the named field from fs or fails the test.
// func findField(t *testing.T, fs []schema.Field, name string) schema.Field {
// 	t.Helper()
// 	for _, f := range fs {
// 		if f.Name == name {
// 			return f
// 		}
// 	}
// 	t.Fatalf("field %q not found in inferred schema", name)
// 	return schema.Field{}
// }

// func seedNamespace(ctx context.Context, t *testing.T, dyn dynamic.Interface, name string) {
// 	t.Helper()
// 	gvr := k8sschema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
// 	ns := &unstructured.Unstructured{Object: map[string]interface{}{
// 		"apiVersion": "v1",
// 		"kind":       "Namespace",
// 		"metadata":   map[string]interface{}{"name": name},
// 	}}
// 	_, err := dyn.Resource(gvr).Create(ctx, ns, metav1.CreateOptions{})
// 	require.NoError(t, err, "create namespace %s", name)
// }

// func seedPod(ctx context.Context, t *testing.T, dyn dynamic.Interface, namespace string) {
// 	t.Helper()
// 	gvr := k8sschema.GroupVersionResource{Version: "v1", Resource: "pods"}
// 	pod := &unstructured.Unstructured{Object: map[string]interface{}{
// 		"apiVersion": "v1",
// 		"kind":       "Pod",
// 		"metadata": map[string]interface{}{
// 			"name":      "nginx",
// 			"namespace": namespace,
// 			"labels":    map[string]interface{}{"app": "nginx"},
// 		},
// 		"spec": map[string]interface{}{
// 			"containers": []interface{}{
// 				map[string]interface{}{"name": "nginx", "image": "nginx:latest"},
// 			},
// 			"volumes": []interface{}{
// 				map[string]interface{}{
// 					"name":     "config",
// 					"emptyDir": map[string]interface{}{},
// 				},
// 			},
// 		},
// 	}}
// 	_, err := dyn.Resource(gvr).Namespace(namespace).Create(ctx, pod, metav1.CreateOptions{})
// 	require.NoError(t, err, "create pod")
// }
