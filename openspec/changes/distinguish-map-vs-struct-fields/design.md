## Context

`internal/port/schema` has one object kind, `FieldTypeObject`, used for both fixed-schema objects (`metadata`, `spec`, `status`) and open-ended `map[string]T` fields (`labels`, `annotations`). octosql, the query engine, has only Struct/List/Tuple — no native map — and its `->` operator (`ObjectFieldAccess`) resolves only against a Struct with the accessed field declared. The previous change (`refactor-schema-defaults-merge`) typed maps as structs whose subfields were sample-discovered keys, which conflates a dynamic map with a fixed contract.

## Goals / Non-Goals

**Goals:**
- Represent maps and structs as distinct kinds in the schema model.
- Infer the distinction from OpenAPI (`additionalProperties` vs `properties`) and from the default baseline (`labels`/`annotations` are maps).
- Preserve each row's actual map keys (no fixed sample-key contract) and keep `keys()` / `contains()` / `length()` working on maps.
- Access an arbitrary map key with `map['key']` syntax.
- Stop expanding a map's transient keys into top-level columns for `DESCRIBE TABLE` / `SELECT *`.

**Non-Goals:**
- Introducing a native map type into octosql.
- Changing list/struct typing from the prior change.

## Decisions

> **Revision (supersedes the original struct-from-sample-keys decision).** The first design materialized a map as an octosql `Struct` over sample-discovered keys. This is **broken** for the multi-row reality: an octosql `Struct` value is purely positional and its field **names live in the column type**, which is fixed at plan time. So every row is forced into one fixed key shape — a row with `{tier,env,vendor}` got crammed into another row's `{app,tier}`, dropping keys and inventing nulls (`TestMapField` proved this). octosql has no map type, and `->` panics at typecheck on any key not in the fixed struct type ([logical.go ObjectFieldAccess.Typecheck](../../../external/octosql/logical/logical.go)), so a struct cannot represent a dynamic map. The decisions below replace that approach.

**`FieldTypeMap` → octosql String holding the row's JSON object.** At the octosql boundary (`fieldToOctoType`) a map column is typed `octosql.String`; each row's value is the JSON-object encoding of that row's actual map (`anyToMapValue`). This is the only representation that preserves per-row keys (struct can't; List-of-pairs renders as an array, not an object).

**Key access via `map['key']`, rewritten to `map_get(map,'key')`.** Native `->` can't index a string/dynamic map, so map-key access uses bracket syntax. The (schema-blind) rewriter turns `path['key']` into `map_get(path, 'key')`, converting the path's dots to arrows so a nested map column (`metadata.labels`) resolves through its parent struct first. `map_get` parses the JSON-object string and returns the key's value or NULL.

**`keys()` / `contains()` / `length()` parse the per-row JSON map.** Their String descriptors detect a JSON-object string (`asJSONMap`) and operate on the real per-row map (keys, value membership, key count); a non-object string falls back to ordinary string semantics.

**Rendering.** JSON output decodes a map column's JSON-object string back into a real object so `labels` renders as `{"app":"nginx"}` rather than an escaped string (`valueToNativeTyped`). Only well-formed JSON-object strings are decoded; scalar columns (never starting with `{`) are untouched.

**Where the distinction bites:** `FieldTypeMap` drives `DESCRIBE TABLE` / `SELECT *` (a map is ONE column, typed `map`, never expanded per-key) and the value/rewrite/render behavior above.

**Inference rules.**
- OpenAPI: `type: object` (or `$ref`) with `AdditionalProperties != nil` and `len(Properties) == 0` → `FieldTypeMap`; with `Properties` → `FieldTypeObject`.
- Default baseline: `labels`, `annotations` → `FieldTypeMap`; `metadata`, `spec`, `status` → `FieldTypeObject`.
- Sample walk: a `map[string]interface{}` whose values are all scalars of one type is heuristically a map only when the field is already known to be a map from a higher-priority layer; otherwise the merge keeps the map kind set by default/OpenAPI. (Merge prefers the map/struct kind from the authoritative layer; see merge note below.)

**Merge interaction.** `mergeSchemas` treats `FieldTypeMap` and `FieldTypeObject` as the same structural family for recursion, but a field's kind, once set by the default baseline or OpenAPI, SHALL NOT be downgraded by a later sample layer (a sample that sees `labels` as a plain object must not turn the map into a struct).

## Risks / Trade-offs

- [`keys`/`length`/`contains`/`map_get` disambiguate map vs plain string by "is it a JSON object string"] → a genuine string column whose value starts with `{` and parses as JSON would be treated as a map. k8s scalar fields don't look like that; documented heuristic.
- [Any key access must use `map['key']`, not `map->key`] → intentional: `->` is struct access, `['key']` is map access. e2e scenarios updated accordingly.
- [JSON output decodes any JSON-object string column] → only columns whose value is a well-formed JSON object are decoded; scalars are untouched.

## Migration Plan

No data migration. `DESCRIBE TABLE pods` output changes (fewer columns: no per-label-key columns). Rollback = revert; the prior all-struct behavior is preserved in history.
