## Why

`FieldTypeMap` columns (`labels`, `annotations`, ConfigMap `data`, etc.) are currently carried as
JSON-object **strings** (`anyToMapValue` in
[`internal/adapter/sql/octosql/database.go`](../../../internal/adapter/sql/octosql/database.go)).
Every map operation (`map_get`, `map_contains_key`, `map_values`, `keys`, `contains`, `length`)
re-parses that JSON string on every call via `asJSONMap`, and every value coming out of a map
(`map_get`, `map_values`) is forced back into a string — losing the value's real type (a numeric
or boolean map value renders as its JSON text, not as `Int`/`Boolean`). The disambiguation between
"a map column" and "a plain string that happens to start with `{`" is also a heuristic
(`asJSONMap` checks the first non-space byte is `{` and that it parses as JSON).

## What Changes

- Represent a `FieldTypeMap` row value as an octosql **`List` of `Any`-typed elements**, laid out
  as alternating key/value pairs: `["key1", val1, "key2", val2, ...]`. Keys are always
  `octosql.String`; each value is wrapped via the existing `anyToOctoValue` converter (the same
  one used for scalar fields), so it becomes its **native octosql type**
  (`Int`, `Float`, `Boolean`, `String`, `Time`, or — for nested objects/arrays — a JSON-string
  fallback, same as today's `FieldTypeList` element encoding).
- `fieldToOctoType` types `FieldTypeMap` columns as `octosql.Type{TypeID: TypeIDList, List: {Element: &octosql.Any}}` instead of `octosql.String`.
- Rewrite `anyToMapValue` to build this flat typed list instead of a JSON string.
- Rewrite `map_get`, `map_contains_key`, `map_values`, `keys`, `contains`, `length` to operate on
  the flat `List` shape (linear scan over even/odd indices) instead of `asJSONMap`. Remove
  `asJSONMap` and `jsonValueToString`.
  - `map_get(map, key)` returns the value **as its stored type**. Since k8s `labels`/
    `annotations`/ConfigMap `data` are all `map[string]string`, most values stay `String` — but a
    value that looks like an RFC3339 timestamp now decodes to `Time` (via `anyToOctoValue`,
    the same converter used for scalar fields), instead of staying a JSON string as it does
    today.
  - `map_contains_key`, `keys`, `contains`, `length` behavior is unchanged from the user's
    perspective (same results), just implemented over the list.
- Update the renderer (`render.go`): `valueToNativeTyped`/`valueToNative` decode a map column
  (`TypeIDList` with `Any` element / our flat key-value shape) back into a JSON object
  `{"key1": val1, ...}` for JSON output, replacing `decodeJSONObject`.
- Update all tests that assert on the JSON-string map encoding
  (`functions_test.go`, `database_test.go`, `map_query_test.go`, e2e `map.feature`,
  `integration.feature`) to assert on the new typed-list shape and typed `map_get` results.

## Capabilities

### Modified Capabilities
- `dynamic-schema`: `FieldTypeMap` columns are typed as `List<Any>` (flat key/value pairs) instead
  of `String` (JSON object); `map_get`/`map_values` return the value's native octosql type.

## Impact

- Code: `internal/adapter/sql/octosql/database.go` (`fieldToOctoType`, `anyToMapValue`,
  `resolveStructValue`, `resolveMapAsStruct`), `functions.go` (`map_get`, `map_contains_key`,
  `map_values`, `keys`, `contains`, `length`), `render.go` (`valueToNativeTyped`, `valueToNative`).
- Tests: `functions_test.go`, `internal/adapter/sql/octosql/database_test.go`,
  `internal/adapter/sql/octosql/map_query_test.go`, `test/e2e/features/map.feature`,
  `test/e2e/features/integration.feature`.
- Behavior change: a map value that looks like an RFC3339 timestamp (e.g.
  `kubectl.kubernetes.io/restartedAt` annotation) now decodes via `map_get`/`map_values`/JSON
  output as a `Time` value instead of a JSON string — same `anyToOctoValue` heuristic already
  applied to every other scalar field. JSON rendering of map columns is otherwise unchanged
  (`{"app":"nginx"}`). No new dependencies — `octosql.Any` is an existing octosql type already
  used for function argument types in this codebase.
