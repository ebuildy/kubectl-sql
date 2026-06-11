## Context

`distinguish-map-vs-struct-fields` (archived/implemented) typed `FieldTypeMap` columns as
`octosql.String` carrying the row's map as a JSON object, with `map_get`/`keys`/`contains`/
`length`/`map_values` detecting "is this string a JSON object" (`asJSONMap`) and parsing it on
every call. This works but:

- Re-parses JSON on every function call against a map column.
- Forces every map value through `jsonValueToString` — a numeric or boolean label/annotation
  value loses its type and becomes its JSON text.
- The map-vs-plain-string disambiguation is a runtime heuristic on the string's content.

This change replaces the JSON-string encoding with a flat `octosql.List` of `octosql.Any`-typed
elements, alternating key and value: `[k1, v1, k2, v2, ...]`. The `FieldTypeMap` column type
becomes `List<Any>`. This is a value-representation change only — the schema-level distinction
between `FieldTypeMap` and `FieldTypeObject` (from the prior change) is unchanged.

## Goals / Non-Goals

**Goals:**
- Carry each map value as its real octosql type (`Int`, `Float`, `Boolean`, `String`) rather than
  always a string.
- Avoid JSON parsing on every `map_get`/`keys`/`contains`/`length`/`map_values` call — operate
  directly on the `List` structure.
- Keep `map['key']` syntax (rewritten to `map_get(map, 'key')`) and all existing map function
  names working with the same query-level semantics.
- Keep JSON rendering of a map column as a JSON object (`{"app":"nginx"}`), unchanged from the
  user's perspective.

**Non-Goals:**
- Introducing a native octosql map type (still doesn't exist; `List<Any>` is the closest fit).
- Changing `FieldTypeList` (array) or `FieldTypeObject` (struct) encoding.
- Changing the schema-level `FieldTypeMap` vs `FieldTypeObject` inference rules.

## Decisions

**Representation: `List<Any>` of alternating key/value elements.**
`octosql.Type` has no map type. Of the two candidates:
- `Tuple` requires a *fixed arity* known at typecheck time — wrong for a per-row varying number
  of keys.
- `List` requires one `Element` type for static typing, but `octosql.Any` (`TypeIDAny`, an
  existing type already used for function argument types in `external/octosql/functions/functions.go`
  and `aggregates/count.go`) lets each element hold a value of any concrete `TypeID` while the
  list itself stays variable-length.

So `fieldToOctoType` for `FieldTypeMap` becomes:

```go
case internalschema.FieldTypeMap:
	return octosql.Type{
		TypeID: octosql.TypeIDList,
		List:   struct{ Element *octosql.Type }{Element: &octosql.Any},
	}
```

A row value is `octosql.NewList([]octosql.Value{key1, val1, key2, val2, ...})` where each `keyN`
is `octosql.NewString(...)` and each `valN` is built via `anyToOctoValue` (the existing scalar
converter already used for non-map fields), preserving `Int`/`Float`/`Boolean`/`String`. Nested
objects/arrays as map values fall back to a JSON-string element, matching how `FieldTypeList`
already encodes composite elements (`anyToListValue`).

**`anyToMapValue` rewrite.** Replace the `json.Marshal` body with a loop building the flat
`[]octosql.Value`. Keys are sorted (as today's `sortedKeys` does for `keys()`/`map_values()`) so
output is deterministic.

```go
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
```

`anyToOctoValue` (existing helper) already JSON-encodes `map[string]interface{}`/`[]interface{}`
values to a string — reused as-is for nested-object map values (e.g. ConfigMap `data['config.json']`
holding a JSON blob stays a string, same as today).

**Map functions operate on the flat list directly.**

- `map_get(map, key)`: linear scan over `map.List` in steps of 2; if `map.List[i].Str == key`,
  return `map.List[i+1]` **as-is** (its native type) instead of stringifying. Returns `NULL` if
  not found or if the input isn't a `List` (replaces the `asJSONMap` ok-check).
- `map_contains_key(map, key)`: same scan, return `Boolean` on key match.
- `keys(map)`: every even-indexed element, already `octosql.String`. (The existing struct-typed
  `keys()` descriptor for `TypeIDStruct` is untouched; only the `String`-typed JSON-map
  descriptor is replaced — and since the column is now `TypeIDList`, the descriptor moves from
  matching `TypeIDString` to matching `TypeIDList` with `Element.TypeID == TypeIDAny`.)
- `map_values(map)`: every odd-indexed element, returned in their native types. Return type
  becomes `List<Any>` instead of `List<String>` (callers that need strings can wrap with an
  explicit cast/format, same as any other typed column).
- `contains(map, needle)`: true if any odd-indexed (value) element equals `needle` — reuses the
  existing `anyEqual` helper already used for `TypeIDList`/`TypeIDStruct` descriptors, since
  `map.List` IS a `[]octosql.Value` now. This likely collapses into the existing `TypeIDList`
  descriptor (no separate map case needed) — **but** the existing plain-`TypeIDList` descriptor
  for `FieldTypeList` columns checks every element, which for a map's flat list would also match
  against keys. To preserve "contains checks values, not keys" semantics for maps, the `List`
  descriptor needs to distinguish a `FieldTypeList`-style list (`Element` is concrete, e.g.
  `String`) from a map's flat list (`Element.TypeID == TypeIDAny`): for the latter, only scan
  odd-indexed elements.
- `length(map)`: number of keys = `len(map.List) / 2`. The `TypeIDList` descriptor needs the same
  `Element.TypeID == TypeIDAny` check to divide by 2 only for map columns; a regular
  `FieldTypeList` column's `length()` stays `len(list)`.

**Disambiguating "map list" vs "regular list" at the type level.** Both are `TypeIDList`, but a
map's `Element` type is `octosql.Any` (`TypeIDAny`) while a `FieldTypeList` column's `Element` is
always `octosql.String` (per `fieldToOctoType`'s existing `FieldTypeList` case). Function
descriptors use `types[0].List.Element.TypeID == octosql.TypeIDAny` as the map/regular-list
discriminator. This is a static (typecheck-time) check, not a runtime heuristic — strictly more
robust than `asJSONMap`'s "does this string parse as `{...}`" check.

**Renderer.** `valueToNativeTyped`/`valueToNative`: a `TypeIDList` value whose type's `Element` is
`TypeIDAny` is decoded into a `map[string]interface{}` (even elements → keys, odd → native
values via the existing `valueToNative`) instead of a passthrough list. Replaces
`decodeJSONObject`. Table/CSV rendering (`valueToString`) for a map column renders it via
`v.String()` (octosql's default `List` stringification) — acceptable since table cells already
truncate; if this looks wrong in practice we can special-case it, but it's out of scope for
correctness.

## Risks / Trade-offs

- **`map_get` return type is now a union/Any, not always String.** Callers doing
  `map_get(labels,'app') = 'nginx'` still work (string = string), since k8s `labels`/
  `annotations`/ConfigMap `data` are all `map[string]string` and `anyToOctoValue` keeps plain
  strings as `String`. The one observable change: a value that looks like an RFC3339 timestamp
  (e.g. a `restartedAt` annotation) now decodes to `Time` instead of staying a JSON string —
  consistent with how every other scalar field is already typed via `anyToOctoValue`. Flagged in
  the proposal's Impact section.
- **`TypeIDAny` is marked `// TODO: Remove this type?` in `external/octosql/octosql/types.go`.**
  It's vendored/external code we don't control. If a future octosql upgrade removes `TypeIDAny`,
  this representation needs revisiting. Accepted: it's the only currently-available type that
  fits, and it's already used elsewhere in this codebase for function signatures.
- **`length`/`contains`/`keys` descriptors must branch on `Element.TypeID` to tell a map's flat
  list apart from a `FieldTypeList` column.** Slightly more complex `TypeFn` logic, but it's a
  static type check, not a runtime value inspection — more robust than the string heuristic it
  replaces.
- **`map_values()`'s declared return type uses a `List` with `Element: nil`** (rather than
  `List<Any>`) so the renderer's `isMapListType` check doesn't mistake its plain value list for a
  map column's flat key/value list — both would otherwise be `List<Any>` and indistinguishable at
  render time.
- **Pre-existing bug fixed as a prerequisite**: `rewriteDottedFields` (the regex pass that
  rewrites `field['key']` to `map_get(field, 'key')` and dotted paths to arrow chains) was written
  and unit-tested but never wired into `rewriteQuery` — without it, bracket map-key-access queries
  failed to parse entirely (`invalid argument syntax error`), independent of this change's map
  representation. Re-enabled it, and additionally fixed it to protect string literals (e.g.
  `'config.json'`) from the dotted-path rewrites, which previously corrupted dotted map keys used
  as `map_get` arguments.

## Migration Plan

No data migration (in-memory representation only). `DESCRIBE TABLE` reports
`internalschema.FieldType` (`"map"`) directly, not the octosql column type, so its output is
unaffected by this change. JSON output of map columns is unchanged. Rollback = revert; the
JSON-string representation remains in git history.
