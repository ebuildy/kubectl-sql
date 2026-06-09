## Why

The schema model collapses two genuinely different object shapes into a single `FieldTypeObject`:

1. **Struct** — a fixed, known schema (`metadata`, `spec`, `status`): a bounded set of named fields, knowable up-front from OpenAPI. You access declared fields like `metadata->name`.
2. **Map** — an open-ended `map[string]T` (`labels`, `annotations`): keys vary per object and are only discoverable by sampling. The interesting operations are key enumeration (`keys()`), membership (`contains()`), and lookup of an arbitrary key.

Treating a map as a struct means its sample-discovered keys get presented as if they were a fixed contract: `DESCRIBE TABLE` and `SELECT *` would surface ephemeral label keys as columns, the schema churns per sample, and the model misrepresents `labels`/`annotations`. We need the schema to distinguish the two.

## What Changes

- Add a `schema.FieldTypeMap` kind distinct from `FieldTypeObject` (struct).
- Infer maps vs structs:
  - **OpenAPI**: `type: object` with `additionalProperties` and no `properties` → map; with `properties` → struct.
  - **Sample/default baseline**: `labels` and `annotations` are maps; `metadata`/`spec`/`status` are structs.
- Map fields do NOT contribute their sample-discovered keys as fixed struct subfields in the schema contract; they are typed as maps with a known value type. Struct fields keep their named subfields.
- `DESCRIBE TABLE` / `SELECT *` SHALL NOT expand a map's dynamic keys into columns; a map renders as a single column.
- `keys(map)` and `contains(map, key)` SHALL operate on maps; `metadata->labels->'app'` style lookup continues to resolve a single key.
- Update the `dynamic-schema` spec to describe both kinds explicitly.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `dynamic-schema`: Split the single "object → struct" rule into two — fixed-schema **struct** fields vs open-ended **map** fields — and define inference, typing, column expansion, and `keys()`/`contains()` behavior for each.

## Impact

- Code: `internal/port/schema/` (new `FieldTypeMap`, walk inference), `internal/adapter/datasources/k8s/schema_openapi.go` (additionalProperties detection) and `schema_default.go`, `internal/adapter/sql/octosql/database.go` (map → octosql type + value resolution), `functions.go` (`keys`/`contains` on map), renderer.
- Behavior: `DESCRIBE TABLE pods` stops listing transient label keys; `labels`/`annotations` are maps. Read-only; no new dependencies.
