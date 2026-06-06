## Context

`GetTable` is called once per query by octosql during the typecheck phase, before any records are streamed. It returns a `physical.Schema` that octosql uses to validate column references and plan the query.

Schema inference is a distinct concern with its own port and two adapters wired in priority order:

1. **`OpenAPIInferrer`** (primary) — fetches the cluster's OpenAPI v3 document and derives the field list from the resource schema.
2. **`SampleInferrer`** (fallback) — fetches one object via `LIST limit=1` and walks its top-level keys.
3. **`CompositeInferrer`** — tries primary, merges secondary SubFields into primary object fields that returned without SubFields (e.g. unresolved `$ref` in OpenAPI).

See [ADR-001](../../../docs/adr-001-schema-inference-strategy.md) for the rationale.

## Goals / Non-Goals

**Goals:**
- `SchemaInferrer` interface as the port — all consumers depend on it
- `OpenAPIInferrer` primary, `SampleInferrer` fallback, `CompositeInferrer` wires both
- Object columns use `octosql.TypeIDStruct` — no JSON string serialization for maps
- Nested field access via `->` operator: `metadata->labels->app`
- Dot-notation rewriter converts `metadata.labels.app` → `metadata->labels->app`
- No synthetic flattened alias columns (`metadata_labels`, `metadata_labels_app`)
- Guaranteed fields (`name`, `namespace`) always present

**Non-Goals:**
- Schema caching between queries (future — see ADR-001)
- `--schema-strategy` flag (future)
- Slice element type inference (slices remain JSON strings)

## Architecture

```
internal/
  schema/
    port.go                 ← SchemaInferrer interface + Field/FieldType types
    composite_inferrer.go   ← merges primary + secondary SubFields
    openapi_inferrer.go     ← OpenAPIInferrer (primary)
    sample_inferrer.go      ← SampleInferrer (fallback)
    walk.go                 ← walkObject: top-level fields + SubFields (no aliases)
  executor/
    executor.go             ← receives SchemaInferrer; builds TypeIDStruct for object fields
                               Materialize uses pruned sch.Fields for row ordering
    resolver.go
  k8s/
    client.go
  output/
    renderer.go

cmd/
  root.go                   ← wires CompositeInferrer
                               rewriteDottedFields: metadata.labels.app → metadata->labels->app
                               DESCRIBE TABLE uses SchemaInferrer directly
```

## Port: `SchemaInferrer`

```go
type SchemaInferrer interface {
    InferFields(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]Field, error)
}

type Field struct {
    Name      string
    Path      string    // dot-notation resolve path; empty means same as Name
    Type      FieldType
    SubFields []Field   // populated for FieldTypeObject map fields
}

type FieldType string
const (
    FieldTypeString FieldType = "string"
    FieldTypeInt    FieldType = "int"
    FieldTypeFloat  FieldType = "float"
    FieldTypeBool   FieldType = "bool"
    FieldTypeObject FieldType = "object" // maps → TypeIDStruct; slices → JSON string
)
```

## Dot-notation rewriter

`rewriteDottedFields` in `cmd/root.go` rewrites dot-separated field paths to `->` chains:

```
metadata.labels.app   →  metadata->labels->app
metadata.labels       →  metadata->labels
metadata.labels.*     →  metadata->labels          (wildcard → parent struct)
status.phase          →  status->phase
k8s.pods              →  unchanged (table qualifier)
```

This replaces the previous underscore rewriter. The `->` operator is native to octosql's sqlparser and maps directly to `ObjectFieldAccess` in the logical plan.

## Struct type building (`executor`)

`fieldToOctoType` builds `octosql.Type{TypeID: TypeIDStruct}` from `SubFields`:

```go
case FieldTypeObject:
    if len(f.SubFields) == 0 {
        return octosql.String // slice or empty map — JSON string
    }
    structFields := make([]octosql.StructField, len(f.SubFields))
    for i, sf := range f.SubFields {
        structFields[i] = octosql.StructField{Name: sf.Name, Type: fieldToOctoType(sf)}
    }
    return octosql.Type{TypeID: octosql.TypeIDStruct, Struct: ...}
```

## Struct value production (`Run`)

For struct-typed fields, `resolveStructValue` builds `octosql.NewStruct(values)` with values positionally matching SubFields. Missing subfields are padded with `octosql.NewNull()`.

The `Materialize` method uses `sch.Fields` (the optimizer-pruned field list) for row ordering, looking up `Path` and `SubFields` from the full inferred field list via a name→Field map.

## walk.go

`walkObject` produces top-level fields with `SubFields` for map values. **No flattened alias columns are emitted.** The composite inferrer merges SubFields from the sample into OpenAPI fields that came back without them (unresolved `$ref`).

## Type mapping

| Go / OpenAPI type | schema.FieldType | octosql type |
|---|---|---|
| `string` | `FieldTypeString` | `octosql.String` |
| `bool` | `FieldTypeBool` | `octosql.Boolean` |
| `integer` | `FieldTypeInt` | `octosql.Int` |
| `number` / `float64` | `FieldTypeFloat` | `octosql.Float` |
| `object` / `map[string]interface{}` (with subfields) | `FieldTypeObject` | `octosql.TypeIDStruct` |
| `object` / `map[string]interface{}` (no subfields) | `FieldTypeObject` | `octosql.String` (JSON) |
| `array` / `[]interface{}` | `FieldTypeObject` | `octosql.String` (JSON) |
| `nil` / unknown | `FieldTypeString` | `octosql.String` |

## Risks

- **Unresolved `$ref` in OpenAPI**: `CompositeInferrer` fills in SubFields from sample. Bounded to resources whose OpenAPI schema uses `$ref` without inline properties.
- **Struct field count mismatch at runtime**: pad with `octosql.NewNull()`.
- **OpenAPI latency**: ~100–200 ms first query. Future: cache per connection.
