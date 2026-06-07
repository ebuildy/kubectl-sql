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
	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	internalschema "github.com/ebuildy/kubectl-sql/internal/schema"
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
	inferrer  internalschema.SchemaInferrer
}

// NewKubernetesDatabase creates a new KubernetesDatabase.
func NewKubernetesDatabase(client dynamic.Interface, mapper meta.RESTMapper, namespace string, pageSize int, inferrer internalschema.SchemaInferrer) *KubernetesDatabase {
	return &KubernetesDatabase{
		client:    client,
		mapper:    mapper,
		namespace: namespace,
		pageSize:  int64(pageSize),
		inferrer:  inferrer,
	}
}

// ListTables returns an empty list — table names are resolved dynamically.
func (db *KubernetesDatabase) ListTables(_ context.Context) ([]string, error) {
	return nil, nil
}

// GetTable resolves a resource kind to a GVR, infers the schema via the SchemaInferrer,
// and returns the datasource implementation.
func (db *KubernetesDatabase) GetTable(ctx context.Context, name string, _ map[string]string) (physical.DatasourceImplementation, physical.Schema, error) {
	gvr, err := db.mapper.ResourceFor(schema.GroupVersionResource{Resource: name})
	if err != nil {
		return nil, physical.Schema{}, fmt.Errorf("executor: resolve resource %q: %w", name, err)
	}

	var inferredFields []internalschema.Field
	if db.inferrer != nil {
		inferredFields, _ = db.inferrer.InferFields(ctx, gvr)
	}
	if len(inferredFields) == 0 {
		inferredFields = guaranteedSchemaFields()
	}

	impl := &kubernetesDatasource{
		client:    db.client,
		gvr:       gvr,
		namespace: db.namespace,
		pageSize:  db.pageSize,
		fields:    inferredFields,
	}

	sch := physical.Schema{
		TimeField:     -1,
		NoRetractions: true,
		Fields:        toOctoFields(inferredFields),
	}

	return impl, sch, nil
}

// guaranteedSchemaFields returns the static fallback field list.
func guaranteedSchemaFields() []internalschema.Field {
	return []internalschema.Field{
		{Name: "name", Type: internalschema.FieldTypeString},
		{Name: "namespace", Type: internalschema.FieldTypeString},
	}
}

func toOctoFields(fields []internalschema.Field) []physical.SchemaField {
	out := make([]physical.SchemaField, len(fields))
	for i, f := range fields {
		out[i] = physical.SchemaField{Name: f.Name, Type: fieldToOctoType(f)}
	}
	return out
}

// fieldToOctoType converts a schema.Field (including SubFields) to an octosql.Type.
func fieldToOctoType(f internalschema.Field) octosql.Type {
	switch f.Type {
	case internalschema.FieldTypeBool:
		return octosql.Boolean
	case internalschema.FieldTypeInt:
		return octosql.Int
	case internalschema.FieldTypeFloat:
		return octosql.Float
	case internalschema.FieldTypeObject:
		if len(f.SubFields) == 0 {
			return octosql.String // slice or empty map — serialize as JSON
		}
		structFields := make([]octosql.StructField, len(f.SubFields))
		for i, sf := range f.SubFields {
			structFields[i] = octosql.StructField{Name: sf.Name, Type: fieldToOctoType(sf)}
		}
		return octosql.Type{
			TypeID: octosql.TypeIDStruct,
			Struct: struct{ Fields []octosql.StructField }{Fields: structFields},
		}
	default:
		return octosql.String
	}
}

// kubernetesDatasource implements physical.DatasourceImplementation.
type kubernetesDatasource struct {
	client    dynamic.Interface
	gvr       schema.GroupVersionResource
	namespace string
	pageSize  int64
	fields    []internalschema.Field // full inferred schema (for path/SubFields lookup)
}

func (ds *kubernetesDatasource) Materialize(_ context.Context, _ physical.Environment, sch physical.Schema, _ []physical.Expression) (octoexec.Node, error) {
	// Build a lookup map from column name → internalschema.Field for path and SubFields.
	fieldMap := make(map[string]internalschema.Field, len(ds.fields))
	for _, f := range ds.fields {
		fieldMap[f.Name] = f
	}

	// Use the pruned schema from the optimizer (sch.Fields) for row ordering,
	// but look up path/SubFields from the full inferred field list.
	execFields := make([]internalschema.Field, len(sch.Fields))
	for i, sf := range sch.Fields {
		if f, ok := fieldMap[sf.Name]; ok {
			execFields[i] = f
		} else {
			execFields[i] = internalschema.Field{Name: sf.Name, Type: internalschema.FieldTypeString}
		}
	}

	return &kubernetesExecution{
		client:    ds.client,
		gvr:       ds.gvr,
		namespace: ds.namespace,
		pageSize:  ds.pageSize,
		fields:    execFields,
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
	fields    []internalschema.Field // pruned, ordered to match row value positions
}

func (e *kubernetesExecution) Run(execCtx octoexec.ExecutionContext, produce octoexec.ProduceFn, _ octoexec.MetaSendFn) error {
	var continueToken string
	log := logger.FromContext(execCtx.Context)
	ri := e.client.Resource(e.gvr)

	page := 0
	for {
		opts := metav1.ListOptions{Limit: e.pageSize, Continue: continueToken}
		pageStart := time.Now()

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

		log.Debug("listed resource page",
			logger.String("resource", e.gvr.Resource),
			logger.Int("page", page),
			logger.Int("items", len(items)),
			logger.Duration("elapsed", time.Since(pageStart)))
		page++

		for _, raw := range items {
			row := make([]octosql.Value, len(e.fields))
			for j, field := range e.fields {
				row[j] = resolveFieldValue(raw, field)
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

// resolveFieldValue extracts the octosql.Value for a single field from a raw k8s object.
// For struct-typed fields it builds octosql.NewStruct with values positionally matching SubFields.
func resolveFieldValue(raw map[string]interface{}, field internalschema.Field) octosql.Value {
	switch field.Name {
	case "name":
		return anyToOctoValue(ResolveField(raw, "metadata.name"))
	case "namespace":
		return anyToOctoValue(ResolveField(raw, "metadata.namespace"))
	}

	resolvePath := field.Name
	if field.Path != "" {
		resolvePath = field.Path
	}

	if field.Type == internalschema.FieldTypeObject && len(field.SubFields) > 0 {
		return resolveStructValue(raw, resolvePath, field.SubFields)
	}
	return anyToOctoValue(ResolveField(raw, resolvePath))
}

// resolveStructValue builds an octosql.NewStruct value for a map field.
// Values are ordered to match the SubFields slice (struct value ordering contract).
// Sub-struct fields (FieldTypeObject with SubFields) are recursively resolved.
func resolveStructValue(raw map[string]interface{}, path string, subFields []internalschema.Field) octosql.Value {
	parent := ResolveField(raw, path)
	parentMap, ok := parent.(map[string]interface{})
	if !ok {
		nulls := make([]octosql.Value, len(subFields))
		for i := range nulls {
			nulls[i] = octosql.NewNull()
		}
		return octosql.NewStruct(nulls)
	}

	values := make([]octosql.Value, len(subFields))
	for i, sf := range subFields {
		v, exists := parentMap[sf.Name]
		if !exists {
			values[i] = octosql.NewNull()
			continue
		}
		if sf.Type == internalschema.FieldTypeObject && len(sf.SubFields) > 0 {
			// Recursively build a struct for nested map subfields.
			if nested, ok := v.(map[string]interface{}); ok {
				values[i] = resolveMapAsStruct(nested, sf.SubFields)
			} else {
				values[i] = octosql.NewNull()
			}
		} else {
			values[i] = anyToOctoValue(v)
		}
	}
	return octosql.NewStruct(values)
}

// resolveMapAsStruct converts a raw Go map to an octosql.NewStruct using the given subfield list.
func resolveMapAsStruct(m map[string]interface{}, subFields []internalschema.Field) octosql.Value {
	values := make([]octosql.Value, len(subFields))
	for i, sf := range subFields {
		v, exists := m[sf.Name]
		if !exists {
			values[i] = octosql.NewNull()
		} else {
			values[i] = anyToOctoValue(v)
		}
	}
	return octosql.NewStruct(values)
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
