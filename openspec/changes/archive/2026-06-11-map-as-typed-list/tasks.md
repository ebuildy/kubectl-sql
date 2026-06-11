## 1. Type and value representation

- [x] 1.1 `fieldToOctoType` (`internal/adapter/sql/octosql/database.go`): type `FieldTypeMap` as `octosql.Type{TypeID: TypeIDList, List: {Element: &octosql.Any}}` instead of `octosql.String`
- [x] 1.2 Rewrite `anyToMapValue` to build a flat `octosql.NewList([]octosql.Value{key1, val1, key2, val2, ...})` with sorted keys (reuse `sortedKeys`), each value via `anyToOctoValue`; nil/non-map input yields `octosql.NewList(nil)`
- [x] 1.3 Update `resolveStructValue` and `resolveMapAsStruct` (the `FieldTypeMap` subfield cases) — confirm they already delegate to `anyToMapValue` and need no further change beyond 1.2

## 2. Map functions (`internal/adapter/sql/octosql/functions.go`)

- [x] 2.1 Remove `asJSONMap` and `jsonValueToString`
- [x] 2.2 `map_get(map, key)`: `TypeFn` matches `types[0]` = `List<Any>`, `types[1]` = `String`; `Function` linear-scans `v[0].List` in steps of 2, returns `v[0].List[i+1]` as-is on key match (native type), else `NULL`. Output type is `TypeSum(octosql.Any, octosql.Null)` (or equivalent union)
- [x] 2.3 `map_contains_key(map, key)`: same scan shape, returns `Boolean`
- [x] 2.4 `map_values(map)`: returns `List<Any>` of every odd-indexed element (in key order, since `anyToMapValue` already sorts keys) — update return type from `List<String>` to `List<Any>`
- [x] 2.5 `keys(map)`: add/extend a `TypeIDList`-with-`Any`-element descriptor returning every even-indexed element as `List<String>`; keep the existing `TypeIDStruct` descriptor for struct columns unchanged
- [x] 2.6 `length(map)`: add a `TypeIDList`-with-`Any`-element descriptor returning `len(v[0].List) / 2`; keep existing `List`/`Tuple`/`Struct`/`String` descriptors for non-map columns unchanged
- [x] 2.7 `contains(map, needle)`: add a `TypeIDList`-with-`Any`-element descriptor that scans only odd-indexed (value) elements via `anyEqual`; keep the existing plain-`List`/`Struct`/`String` descriptors (which scan all elements) for non-map columns unchanged
- [x] 2.8 Add a small shared helper, e.g. `isMapList(t octosql.Type) bool` returning `t.TypeID == octosql.TypeIDList && t.List.Element != nil && t.List.Element.TypeID == octosql.TypeIDAny`, used by 2.5–2.7's `TypeFn`s to discriminate a map list from a regular `FieldTypeList` column

## 3. Renderer (`internal/adapter/sql/octosql/render.go`)

- [x] 3.1 Remove `decodeJSONObject`
- [x] 3.2 `valueToNativeTyped`: when `t` is a map list (`isMapList`, reuse helper from 2.8) and `v.TypeID == TypeIDList`, decode to `map[string]interface{}` — even elements as keys (`.Str`), odd elements via `valueToNative`
- [x] 3.3 `valueToNative`: same map-list decoding for the untyped fallback path (when no schema type is available), detected structurally — list with even length, all even-indexed elements `TypeIDString` is ambiguous with a real `List<String>` of even length, so prefer relying on `valueToNativeTyped`'s typed path; only fall back here if a map value reaches `valueToNative` without its type (confirm whether this path is actually reachable for map columns — if not, leave `valueToNative` unchanged and note it in the PR)
  - Confirmed unreachable for map columns: `renderJSON` always has `len(schemaFields) == len(fields)`, so every column goes through `valueToNativeTyped`. Left `valueToNative` unchanged.

## 4. Tests — unit

- [x] 4.1 `internal/adapter/sql/octosql/functions_test.go`: rewrite `TestMap_MapGet`, `TestMapContainsKey`, `TestMapValues`, `TestKeys_JSONMapString` to build `octosql.NewList([]octosql.Value{...})` inputs (alternating key/value) instead of JSON-string inputs; assert `map_get` returns the value's native `octosql.Value` (e.g. `octosql.NewString(...)`, `octosql.NewTime(...)`)
- [x] 4.2 Add a test for `map_get`/`map_values` returning `Time` for an RFC3339-looking map value
- [x] 4.3 Add/extend `length`/`contains`/`keys` tests covering the new `List<Any>` map descriptors, and confirm existing `FieldTypeList` (`List<String>`) behavior for `length`/`contains` is unchanged
- [x] 4.4 `internal/adapter/sql/octosql/database_test.go` (if it asserts `fieldToOctoType`/`anyToMapValue` shapes): no such assertions exist — unaffected, no change needed
- [x] 4.5 `internal/adapter/sql/octosql/map_query_test.go`: fixed by re-enabling `rewriteDottedFields` in `rewriteQuery` (see Notes below) — `TestMapField` and `TestMapField_AccessKeysContains` now pass against the new `List<Any>` row representation without further changes

## 5. Tests — e2e

- [x] 5.1 `test/e2e/features/map.feature`: `map_get`/`map_values`/`keys`/`config.json` scenarios pass unchanged with the new representation (no edits needed beyond the rewrite-pipeline fixes below)
- [x] 5.2 `test/e2e/features/integration.feature`: no map-related assertions needed updating (DESCRIBE TABLE / length() scenarios pass unchanged)
- [x] 5.3 Re-enabled the three commented-out scenarios in `map.feature` (`WHERE ... labels['app'] = ...`, `SELECT metadata->labels`, `SELECT metadata.labels.*`) — all now pass

### Notes — fixes required to make e2e pass

- **Re-enabled `rewriteDottedFields` in `rewriteQuery`** (`rewrite.go`): it was written and unit-tested but never wired in (commented out). Without it, `field['key']` bracket syntax reached `sqlparser.Parse` directly, which accepts it but cannot round-trip it through `sqlparser.String()` (used for the `k8s.` table-prefix rewrite), causing `invalid argument syntax error at position 26` on any query using bracket map-key access. `rewriteDottedFields` now runs first and rewrites `field['key']` to `map_get(field, 'key')` before `sqlparser` ever sees the bracket form.
- **Fixed `rewriteDottedFields` to protect string literals**: `dottedFieldRe` was rewriting dots inside string literals too (e.g. `map_get(data, 'config.json')` → `map_get(data, 'config->json')`), breaking key lookups with dotted keys. String literals are now extracted to placeholders before the dotted-path regex passes and restored afterward.
- **Fixed `map_values()`'s declared return type**: it was `List<Any>`, identical in shape to a map column's flat `[k1,v1,k2,v2,...]` representation, so the renderer's `isMapListType` check mistakenly decoded `map_values()`'s plain value list as a `{key: value}` object. Changed its declared element type to `nil` (a plain list of unknown element type), which `isMapListType` correctly excludes.

## 6. Verification

- [x] 6.1 `make lint build` — clean, 0 issues
- [x] 6.2 `go test ./... -race -count=1` — all packages pass
- [x] 6.3 `make e2e` / `make e2e-run-fake` (via `go test -tags integration ./test/integration/...`) — 50/50 scenarios pass
