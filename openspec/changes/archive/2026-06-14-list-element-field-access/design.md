## Context

`kubectl-sql` infers a `[]schema.Field` tree for each resource and maps it to octosql types in
`internal/adapter/sql/octosql/database.go`. Today a `FieldTypeList` field is always a childless
leaf: it maps to `octosql.TypeIDList` with a `String` element, and each element is materialized as
a JSON-encoded string (`anyToListValue`). Because the element type is `String`, octosql's `[]`
index operator returns a string, so `spec->containers[0]->name` cannot type-check — `->` requires a
struct.

The swagger generator (`tools/genk8sschema/fields.go`) already resolves `$ref` chains for
object-typed properties into `SubFields` under a depth cap (8) and a cycle guard, but
`schemaToField` returns early for non-object types, so array element schemas are discarded. The
live OpenAPI inferrer (`schema_openapi.go`) has the same gap.

octosql already supports the target access pattern *if the types are declared*: the `[]` operator's
`TypeFn` returns `Null | Element` for a `List<Element>` (functions.go:1014), and `ObjectFieldAccess`
typecheck (`TypecheckPossiblyNullableStruct`) accepts a union containing a struct. So the entire
change is about producing the right schema types and materializing real struct element values — no
parser or octosql-core changes.

## Goals / Non-Goals

**Goals:**
- Resolve array element object schemas into the list field's `SubFields` in the swagger generator.
- Carry list element schema through the schema model and the layer-merge.
- Type a list-with-element-schema as `octosql.TypeIDList` of `TypeIDStruct`, and materialize each
  element as a real struct so `list[i]->field` works in SELECT and WHERE.
- Render `List<Struct>` cells as arrays of named-key objects, preserving the YAML beautify path.
- Extensive unit tests for the swagger loader's nested schema behavior.

**Non-Goals:**
- No new SQL syntax. Index + `->` already parse; only types change.
- No change to the map representation (`List<Any>` flat key/value) or scalar list behavior.
- No predicate pushdown of element access into the k8s API.
- Not regenerating the committed snapshot as part of review — `make generate` is a task step.

## Decisions

### Decision: A `FieldTypeList` field's `SubFields` describe the element schema
Reuse the existing `Field.SubFields` slice rather than adding an `ElementType *Field`. For a
`FieldTypeObject` field `SubFields` are the object's fields; for a `FieldTypeList` field they are
the element object's fields. This keeps the gob wire format and `Field.Child` unchanged and matches
how `LimitDepth`/`MarshalSubFieldsJSON` already recurse.
- *Alternative considered*: dedicated `Element *Field`. Rejected — larger model/serialization
  churn, two recursion shapes to maintain, and DESCRIBE/JSON helpers would need special-casing.

### Decision: Element resolution lives next to object resolution in the generator
In `schemaToField`, when `classify` returns `FieldTypeList`, inspect the schema's `Items`. If the
item is a `$ref`/inline object, recurse with the same `defs`, `depth+1`, and `visiting` cycle guard
to fill the list field's `SubFields`. Scalar/map/absent items leave `SubFields` nil. Mirror this in
`schema_openapi.go`'s `openAPISchemaToField` so non-embedded resources behave identically.

### Decision: Open-ended maps stay `FieldTypeMap` at any depth
The generator's `classify` (and the live inferrer's `openAPITypeToFieldType`) already map an object
with `additionalProperties` and no fixed `properties` to `FieldTypeMap`. Because the new list-element
recursion routes every element property back through `classify`/`openAPISchemaToField`, nested
`map[string]T` fields (e.g. `metadata->labels`, `spec->nodeSelector`, a Container's
`resources->limits`) are classified as maps automatically. The change makes this an explicit,
tested contract rather than new logic: a map MUST NOT be emitted as a childless `FieldTypeObject`,
so it materializes as the existing `List<Any>` flat key/value representation (`anyToMapValue`) and
renders as an object — never as an empty struct.

### Decision: octosql typing keyed on list `SubFields`
In `fieldToOctoType`, the `FieldTypeList` branch becomes: if `len(f.SubFields) > 0`, build a
`Struct` element type from the subfields (reusing the object branch's struct-building); else keep
`String` element as today. The map branch (`Element: Any`) is untouched, preserving the
list-vs-map type distinction the `dynamic-schema` spec relies on.

### Decision: Element value materialization
Generalize `anyToListValue` to take the element field (or its subfields). For each raw slice
element that is a `map[string]interface{}` and the list has element subfields, build a struct via
the existing `resolveMapAsStruct` (positional, recursive, already handles nested list/map/object).
Elements that don't match (non-map, or no subfields) fall back to the current JSON-string encoding,
so a single helper serves both typed and untyped lists. Update the three call sites
(`resolveFieldValue`, `resolveStructValue`, `resolveMapAsStruct`) to pass element subfields.

### Decision: Renderer decodes struct elements via element type
`valueToNativeTyped`'s `TypeIDList` handling currently decodes each element as a JSON string. Add a
branch: when the schema type is `List<Struct>`, decode each element with `valueToNativeTyped(elem,
elementStructType)` so field names resolve. `rendersAsJSON` already returns true for lists, so the
beautify path (YAML default) is reused unchanged — satisfying the "keep YAML beautify" constraint.

## Risks / Trade-offs

- **Schema/cell size growth** (deeply nested element structs, e.g. `containers` → `Container` →
  `ports`/`env`/…) → Mitigation: the existing depth cap (8) and cycle guard bound recursion;
  DESCRIBE already uses `LimitDepth`.
- **Snapshot size grows** once element subfields are emitted → Mitigation: payload is gzipped and
  decoded once; element subfields are the same data the object path already emits, just under lists.
- **octosql strictness**: a list typed `List<Struct{a,b}>` whose runtime element struct has a
  different arity would panic → Mitigation: `resolveMapAsStruct` always emits exactly the declared
  subfields in order (missing keys → NULL), guaranteeing arity match; untyped fallback path
  unaffected.
- **Mixed/heterogeneous list elements** (rare in k8s typed APIs) → Mitigation: each element is
  resolved against the same declared subfields; absent keys become NULL rather than erroring.
- **Regression risk for scalar lists** → Mitigation: behavior is gated on `len(SubFields) > 0`;
  scalar lists keep `List<String>` exactly. Covered by explicit "unchanged" scenarios.

## Migration Plan

1. Generator + model + openapi inferrer changes (pure additive: nil SubFields ⇒ old behavior).
2. octosql typing + value materialization + renderer.
3. `make generate` to refresh the committed snapshot.
4. `make lint build test`. No runtime migration or flag; backward compatible.

## Open Questions

- None blocking. Depth symmetry with objects (cap 8) is assumed acceptable per the proposal
  discussion; revisit only if generated snapshot size becomes a problem.
