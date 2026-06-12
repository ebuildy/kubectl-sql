package octosql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	octoexec "github.com/cube2222/octosql/execution"
	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	internalschema "github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// KubernetesDatabase implements physical.Database — one "database" for the whole cluster.
// It obtains all cluster data through the k8s DataSource port (no client-go here).
type KubernetesDatabase struct {
	ds        k8sport.DataSource
	namespace string
	pageSize  int64
}

// NewKubernetesDatabase creates a new KubernetesDatabase backed by a DataSource port.
func NewKubernetesDatabase(ds k8sport.DataSource, namespace string, pageSize int) *KubernetesDatabase {
	return &KubernetesDatabase{
		ds:        ds,
		namespace: namespace,
		pageSize:  int64(pageSize),
	}
}

// ListTables returns an empty list — table names are resolved dynamically.
func (db *KubernetesDatabase) ListTables(_ context.Context) ([]string, error) {
	return nil, nil
}

// GetTable resolves a resource kind via the port, infers its schema, and returns
// the datasource implementation.
func (db *KubernetesDatabase) GetTable(ctx context.Context, name string, _ map[string]string) (physical.DatasourceImplementation, physical.Schema, error) {
	log := logger.FromContext(ctx)

	resource, err := db.ds.Resolve(ctx, name)
	if err != nil {
		return nil, physical.Schema{}, fmt.Errorf("executor: resolve resource %q: %w", name, err)
	}

	inferredFields, _ := db.ds.InferSchema(ctx, resource)
	if len(inferredFields) == 0 {
		inferredFields = guaranteedSchemaFields()
	}
	if log.TraceEnabled() {
		if raw, err := json.Marshal(inferredFields); err == nil {
			log.Trace("inferred schema", logger.String("table", name), logger.String("schema", string(raw)))
		}
	}

	impl := &kubernetesDatasource{
		ds:        db.ds,
		resource:  resource,
		namespace: db.namespace,
		pageSize:  db.pageSize,
		fields:    inferredFields,
	}

	octoFields := toOctoFields(inferredFields)
	if log.TraceEnabled() {
		if raw, err := json.Marshal(octoFields); err == nil {
			log.Trace("octosql schema", logger.String("table", name), logger.String("schema", string(raw)))
		}
	}

	sch := physical.Schema{
		TimeField:     -1,
		NoRetractions: true,
		Fields:        octoFields,
	}

	return impl, sch, nil
}

// guaranteedSchemaFields returns the static fallback field list.
func guaranteedSchemaFields() []internalschema.Field {
	return []internalschema.Field{
		{Name: "name", Type: internalschema.FieldTypeString},
		{Name: "namespace", Type: internalschema.FieldTypeString},
		{Name: "labels", Type: internalschema.FieldTypeMap},
		{Name: "annotations", Type: internalschema.FieldTypeMap},
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
	case internalschema.FieldTypeList:
		// Lists carry JSON-string elements so length() counts elements while each
		// element still renders as its JSON form.
		elem := octosql.String
		return octosql.Type{
			TypeID: octosql.TypeIDList,
			List:   struct{ Element *octosql.Type }{Element: &elem},
		}
	case internalschema.FieldTypeMap:
		// An open-ended map[string]T has per-row varying keys. octosql has no map type
		// and its Struct is a FIXED, positional shape (names live in the type, not the
		// value), so a struct cannot represent a dynamic map. We carry the map as a flat
		// List<Any> of alternating key/value elements ([k1, v1, k2, v2, ...]), each value
		// in its native octosql type; key access is map['key'] (rewritten to map_get),
		// and keys()/contains()/length() operate on the flat list. The renderer decodes
		// map columns back into JSON objects.
		return octosql.Type{
			TypeID: octosql.TypeIDList,
			List:   struct{ Element *octosql.Type }{Element: &octosql.Any},
		}
	case internalschema.FieldTypeObject:
		// A fixed-schema struct materializes as an octosql Struct over its known
		// subfields, so -> access works.
		if len(f.SubFields) == 0 {
			return octosql.String // no known keys yet — serialize as JSON
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
	ds        k8sport.DataSource
	resource  k8sport.Resource
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
		ds:        ds.ds,
		resource:  ds.resource,
		namespace: ds.namespace,
		pageSize:  ds.pageSize,
		fields:    execFields,
	}, nil
}

func (ds *kubernetesDatasource) PushDownPredicates(newPredicates, _ []physical.Expression) ([]physical.Expression, []physical.Expression, bool) {
	return newPredicates, nil, false
}

// kubernetesExecution implements execution.Node — streams k8s resources as rows
// obtained through the DataSource port.
type kubernetesExecution struct {
	ds        k8sport.DataSource
	resource  k8sport.Resource
	namespace string
	pageSize  int64
	fields    []internalschema.Field // pruned, ordered to match row value positions
}

func (e *kubernetesExecution) Run(execCtx octoexec.ExecutionContext, produce octoexec.ProduceFn, _ octoexec.MetaSendFn) error {
	opts := k8sport.ListOptions{Namespace: e.namespace, PageSize: e.pageSize}
	return e.ds.List(execCtx.Context, e.resource, opts, func(page []map[string]any) error {
		for _, raw := range page {
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
		return nil
	})
}

// resolveFieldValue extracts the octosql.Value for a single field from a raw k8s object.
// For struct-typed fields it builds octosql.NewStruct with values positionally matching SubFields.
func resolveFieldValue(raw map[string]interface{}, field internalschema.Field) octosql.Value {
	switch field.Name {
	case "name":
		return anyToOctoValue(ResolveField(raw, "metadata.name"))
	case "namespace":
		return anyToOctoValue(ResolveField(raw, "metadata.namespace"))
	case "labels":
		return anyToMapValue(ResolveField(raw, "metadata.labels"))
	case "annotations":
		return anyToMapValue(ResolveField(raw, "metadata.annotations"))
	}

	resolvePath := field.Name

	if field.Type == internalschema.FieldTypeList {
		return anyToListValue(ResolveField(raw, resolvePath))
	}
	if field.Type == internalschema.FieldTypeMap {
		return anyToMapValue(ResolveField(raw, resolvePath))
	}
	// A fixed-schema struct materializes as an octosql Struct over its known subfields.
	if field.Type == internalschema.FieldTypeObject && len(field.SubFields) > 0 {
		return resolveStructValue(raw, resolvePath, field.SubFields)
	}
	return anyToOctoValue(ResolveField(raw, resolvePath))
}

// anyToMapValue encodes a map[string]T value as a flat List<Any> of alternating
// key/value elements ([k1, v1, k2, v2, ...]), the runtime representation of a
// FieldTypeMap column. Keys are sorted for deterministic output. Each value is
// converted via anyToOctoValue, preserving its native octosql type. A nil/non-map
// value yields an empty list so the column type stays List<Any>.
func anyToMapValue(v interface{}) octosql.Value {
	m, ok := v.(map[string]interface{})
	if !ok || m == nil {
		return octosql.NewList(nil)
	}
	keys := sortedKeys(m)
	elems := make([]octosql.Value, 0, len(keys)*2)
	for _, k := range keys {
		elems = append(elems, octosql.NewString(k), anyToOctoValue(m[k]))
	}
	return octosql.NewList(elems)
}

// anyToListValue builds an octosql List value whose elements are the JSON-encoded
// form of each slice element. A non-slice (or nil) value yields an empty list so
// the runtime type stays a List, matching the schema declared in fieldToOctoType.
func anyToListValue(v interface{}) octosql.Value {
	slice, ok := v.([]interface{})
	if !ok {
		return octosql.NewList(nil)
	}
	elems := make([]octosql.Value, len(slice))
	for i, e := range slice {
		switch e.(type) {
		case map[string]interface{}, []interface{}:
			b, err := json.Marshal(e)
			if err != nil {
				elems[i] = octosql.NewNull()
				continue
			}
			elems[i] = octosql.NewString(string(b))
		default:
			elems[i] = octosql.NewString(fmt.Sprintf("%v", e))
		}
	}
	return octosql.NewList(elems)
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
		switch {
		case sf.Type == internalschema.FieldTypeList:
			values[i] = anyToListValue(v)
		case sf.Type == internalschema.FieldTypeMap:
			values[i] = anyToMapValue(v)
		case sf.Type == internalschema.FieldTypeObject && len(sf.SubFields) > 0:
			// Recursively build a struct for nested struct subfields.
			if nested, ok := v.(map[string]interface{}); ok {
				values[i] = resolveMapAsStruct(nested, sf.SubFields)
			} else {
				values[i] = octosql.NewNull()
			}
		default:
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
		switch {
		case !exists:
			values[i] = octosql.NewNull()
		case sf.Type == internalschema.FieldTypeList:
			values[i] = anyToListValue(v)
		case sf.Type == internalschema.FieldTypeMap:
			values[i] = anyToMapValue(v)
		case sf.Type == internalschema.FieldTypeObject && len(sf.SubFields) > 0:
			if nested, ok := v.(map[string]interface{}); ok {
				values[i] = resolveMapAsStruct(nested, sf.SubFields)
			} else {
				values[i] = octosql.NewNull()
			}
		default:
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
