// Package executor will run execution plans against the Kubernetes API via octosql.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	octoexec "github.com/cube2222/octosql/execution"
	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// KubernetesDatabase implements physical.Database — one "database" for the whole cluster.
type KubernetesDatabase struct {
	client    dynamic.Interface
	mapper    meta.RESTMapper
	namespace string
	pageSize  int64
}

// NewKubernetesDatabase creates a new KubernetesDatabase.
func NewKubernetesDatabase(client dynamic.Interface, mapper meta.RESTMapper, namespace string, pageSize int) *KubernetesDatabase {
	return &KubernetesDatabase{
		client:    client,
		mapper:    mapper,
		namespace: namespace,
		pageSize:  int64(pageSize),
	}
}

// ListTables returns an empty list — table names are resolved dynamically.
func (db *KubernetesDatabase) ListTables(_ context.Context) ([]string, error) {
	return nil, nil
}

// GetTable resolves a resource kind to a GVR and returns the datasource implementation.
func (db *KubernetesDatabase) GetTable(_ context.Context, name string, _ map[string]string) (physical.DatasourceImplementation, physical.Schema, error) {
	gvr, err := db.mapper.ResourceFor(schema.GroupVersionResource{Resource: name})
	if err != nil {
		return nil, physical.Schema{}, fmt.Errorf("executor: resolve resource %q: %w", name, err)
	}

	impl := &kubernetesDatasource{
		client:    db.client,
		gvr:       gvr,
		namespace: db.namespace,
		pageSize:  db.pageSize,
	}

	sch := physical.Schema{
		TimeField:     -1,
		NoRetractions: true,
		Fields: []physical.SchemaField{
			{Name: "name", Type: octosql.String},
			{Name: "namespace", Type: octosql.String},
			{Name: "raw", Type: octosql.String},
		},
	}

	return impl, sch, nil
}

// kubernetesDatasource implements physical.DatasourceImplementation.
type kubernetesDatasource struct {
	client    dynamic.Interface
	gvr       schema.GroupVersionResource
	namespace string
	pageSize  int64
}

func (ds *kubernetesDatasource) Materialize(_ context.Context, _ physical.Environment, sch physical.Schema, _ []physical.Expression) (octoexec.Node, error) {
	return &kubernetesExecution{
		client:    ds.client,
		gvr:       ds.gvr,
		namespace: ds.namespace,
		pageSize:  ds.pageSize,
		fields:    sch.Fields,
	}, nil
}

func (ds *kubernetesDatasource) PushDownPredicates(newPredicates, _ []physical.Expression) ([]physical.Expression, []physical.Expression, bool) {
	return newPredicates, nil, false
}

// kubernetesExecution implements execution.Node — streams k8s resources as rows.
type kubernetesExecution struct {
	client    dynamic.Interface
	gvr       schema.GroupVersionResource
	namespace string
	pageSize  int64
	fields    []physical.SchemaField
}

func (e *kubernetesExecution) Run(execCtx octoexec.ExecutionContext, produce octoexec.ProduceFn, _ octoexec.MetaSendFn) error {
	var continueToken string
	ri := e.client.Resource(e.gvr)

	for {
		opts := metav1.ListOptions{Limit: e.pageSize, Continue: continueToken}

		var items []map[string]interface{}
		var nextToken string

		if e.namespace != "" {
			list, err := ri.Namespace(e.namespace).List(execCtx.Context, opts)
			if err != nil {
				return fmt.Errorf("executor: list %s: %w", e.gvr.Resource, err)
			}
			for i := range list.Items {
				items = append(items, list.Items[i].Object)
			}
			nextToken = list.GetContinue()
		} else {
			list, err := ri.List(execCtx.Context, opts)
			if err != nil {
				return fmt.Errorf("executor: list %s: %w", e.gvr.Resource, err)
			}
			for i := range list.Items {
				items = append(items, list.Items[i].Object)
			}
			nextToken = list.GetContinue()
		}

		for _, raw := range items {
			row := make([]octosql.Value, len(e.fields))
			for j, field := range e.fields {
				path := field.Name
				// Map well-known short names to their metadata paths.
				switch path {
				case "name":
					path = "metadata.name"
				case "namespace":
					path = "metadata.namespace"
				}
				row[j] = anyToOctoValue(ResolveField(raw, path))
			}
			if err := produce(
				octoexec.ProduceFromExecutionContext(execCtx),
				octoexec.NewRecord(row, false, time.Time{}),
			); err != nil {
				return fmt.Errorf("executor: produce record: %w", err)
			}
		}

		continueToken = nextToken
		if continueToken == "" {
			break
		}
	}
	return nil
}

// anyToOctoValue converts an arbitrary Go value to an octosql.Value.
func anyToOctoValue(v interface{}) octosql.Value {
	if v == nil {
		return octosql.NewNull()
	}
	switch val := v.(type) {
	case bool:
		return octosql.NewBoolean(val)
	case int64:
		return octosql.NewInt(val)
	case int:
		return octosql.NewInt(int64(val))
	case float64:
		if val == float64(int64(val)) {
			return octosql.NewInt(int64(val))
		}
		return octosql.NewFloat(val)
	case string:
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return octosql.NewTime(t)
		}
		return octosql.NewString(val)
	case map[string]interface{}:
		b, err := json.Marshal(val)
		if err != nil {
			return octosql.NewNull()
		}
		return octosql.NewString(string(b))
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return octosql.NewString(fmt.Sprintf("%v", v))
		}
		return octosql.NewString(string(b))
	}
}
