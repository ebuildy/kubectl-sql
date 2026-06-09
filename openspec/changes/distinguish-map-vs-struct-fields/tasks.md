## 1. Schema model

- [x] 1.1 Add `schema.FieldTypeMap` to `internal/port/schema/field.go` with a doc comment distinguishing it from `FieldTypeObject`
- [x] 1.2 Add `FieldType.IsObjectLike()` helper (struct OR map); maps reuse `SubFields` for sample keys

## 2. Inference

- [x] 2.1 OpenAPI: classify `type: object` (or `$ref`) with `AdditionalProperties != nil` and no `Properties` as `FieldTypeMap` (`isOpenAPIMap`)
- [x] 2.2 Default baseline: type `labels`, `annotations` as `FieldTypeMap` (top-level and under `metadata`)
- [x] 2.3 `mergeSchemas`: treat map+struct as same structural family for recursion; never downgrade a kind set by an authoritative layer; sample keys still merge into a map's SubFields

## 3. octosql adapter

- [x] 3.1 `DESCRIBE TABLE` / `SELECT *`: a map renders as a single column (top-level fields only; no per-key expansion) — verified

## 4. Per-row map representation (supersedes the struct-from-sample-keys approach)

> A struct cannot represent per-row varying keys (octosql Struct values are positional with a fixed per-column type). `TestMapField` proved row 2 was crammed into row 1's key shape. Maps are now JSON-object strings with bracket key access.

- [x] 4.1 `fieldToOctoType`: type `FieldTypeMap` as `octosql.String`; structs keep `TypeIDStruct`
- [x] 4.2 Value resolution: emit a map's per-row value as a JSON-object string (`anyToMapValue`) in `resolveFieldValue` and the nested resolvers
- [x] 4.3 Rewriter: `map['key']` → `map_get(map, 'key')` (path dots → arrows); numeric `[N]` unchanged
- [x] 4.4 `map_get(map,key)` function; `keys`/`contains`/`length` detect a JSON-object string and operate on the per-row map
- [x] 4.5 JSON renderer decodes a map column's JSON-object string into a real object

## 5. Tests

- [x] 5.1 Unit: OpenAPI inferrer classifies `additionalProperties` → `FieldTypeMap`, `properties`/`$ref` → `FieldTypeObject`
- [x] 5.2 Unit: default baseline marks `labels`/`annotations` as `FieldTypeMap`, `metadata`/`spec`/`status` as `FieldTypeObject`
- [x] 5.3 Unit: `mergeSchemas` keeps map kind when a sample supplies a struct-shaped object
- [x] 5.4 Unit: rewriter turns `labels['app']` / `metadata.labels['app']` into `map_get(...)`
- [x] 5.5 Unit: `map_get`, and JSON-map-aware `keys`/`contains`/`length`
- [x] 5.6 Engine: per-row dynamic keys — two pods with different label sets keep their own keys; `labels['app']`, `keys`, `contains`, `length` all work; missing key → null
- [x] 5.7 e2e: `DESCRIBE TABLE pods` types `labels` as `map` (no per-key columns); `metadata.labels['app']` bracket access and WHERE filter
- [x] 5.8 Run `make lint build`, `go test ./... -race`, `make e2e`, `make e2e-run-fake` (all pass)
