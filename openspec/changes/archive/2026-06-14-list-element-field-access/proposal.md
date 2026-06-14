## Why

List-typed fields (e.g. `spec->containers`, `status->conditions`) are currently inferred as
opaque `FieldTypeList` leaves with no element schema, so each element materializes as a
JSON-encoded string. This makes the most common Kubernetes debugging queries impossible:
`spec->containers[0]->name` fails because indexing a `List<String>` yields a string, not a
struct with named fields. The swagger snapshot already resolves object `$ref` chains to full
depth — extending that to list element types unlocks nested element access end to end.

## What Changes

- The swagger generator (`tools/genk8sschema`) SHALL resolve the **element type** of array
  properties: when a list's items are a `$ref` to (or inline) an object, its fields are captured
  as the list `Field`'s `SubFields`, recursing under the same depth cap and cycle guard as
  objects today.
- The `schema.Field` model gains a documented meaning for `SubFields` on a `FieldTypeList`
  field: they describe the schema of each list **element** (not subfields of the list itself).
- Schema inference / merge (`dynamic-schema`) SHALL preserve and merge list element `SubFields`
  across the swagger, OpenAPI, and sample layers.
- octosql type mapping SHALL render a `FieldTypeList` with element `SubFields` as
  `List<Struct{…}>` (instead of `List<String>`), and value resolution SHALL materialize each
  element as a real octosql `Struct` so `list[i]->field` type-checks and evaluates.
- The output renderer SHALL render `List<Struct>` cells (table/json/csv) as arrays of objects
  with named keys, decoding struct elements via their element type. In table output these cells
  SHALL continue to flow through the existing **YAML beautify** path
  (`beautifyFormatActive = beautifyFormatYAML`) — i.e. render as pretty YAML, not JSON — exactly
  as object/list cells do today.
- Lists whose elements are scalars or have no resolvable element schema SHALL keep today's
  `List<String>` (JSON-string element) behavior — no regression.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `swagger-schema-provider`: the generator resolves array element types into the list field's
  `SubFields` instead of emitting list fields as childless leaves.
- `dynamic-schema`: the field model and the layer-merge algorithm carry and reconcile list
  element subfields.
- `sql-execution`: list fields with an element schema are typed `List<Struct>`, enabling
  `<list>[index]->field` access; element values materialize as structs.
- `output-renderer`: `List<Struct>` cells render as arrays of named-key objects across all
  output formats, preserving the existing YAML beautify path for table output.

## Impact

- Generator: `tools/genk8sschema/fields.go` (and regenerated
  `internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go` via `make generate`).
- Schema model: `internal/port/schema/field.go`, `internal/port/schema/walk.go` (doc/semantics),
  the merge logic feeding `dynamic-schema`.
- Live OpenAPI inferrer: `internal/adapter/datasources/k8s/schema_openapi.go` (parity for
  non-embedded resources).
- octosql adapter: `internal/adapter/sql/octosql/database.go` (`fieldToOctoType`,
  `anyToListValue` and struct/list value resolution), `internal/adapter/sql/octosql/render.go`
  (`List<Struct>` rendering).
- No new dependencies. Read-only; no change to cluster access patterns.
