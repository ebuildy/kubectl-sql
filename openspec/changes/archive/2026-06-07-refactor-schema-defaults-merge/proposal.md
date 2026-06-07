## Why

Schema inference previously treated OpenAPI as the primary source and a sample object as the fallback, with the two never combined in a structured way. This produced inconsistent column sets: queries like `SELECT * FROM pods` could be missing the well-known top-level fields (`metadata`, `spec`, `status`, plus `name`/`namespace`/`labels`/`annotations`) when the OpenAPI document was unavailable or partial, and there was no single deterministic baseline shared across resources. We need a predictable schema that always starts from a known default and is then enriched, not replaced, by cluster-derived information.

## What Changes

- Introduce a **defaults-first merge** strategy: schema inference now starts from a hardcoded default field set (`name`, `namespace`, `labels`, `annotations`, `metadata`, `spec`, `status`) and then merges richer fields discovered from the OpenAPI v3 schema, falling back to a sample-object inferrer when OpenAPI yields nothing.
- Add a recursive `mergeSchemas` routine that merges a source field list into a destination tree: matching object fields are merged by recursing into their subfields, new fields are appended, and a field-type mismatch between sources is a hard error.
- Split the monolithic `inferrer.go` into focused files: `schema.go` (composite/merge orchestration), `schema_default.go` (default baseline), `schema_openapi.go` (OpenAPI inferrer), `schema_sample.go` (sample inferrer).
- Remove the obsolete `inferrer.go` / `inferrer_test.go` from the k8s datasource adapter.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `dynamic-schema`: The inference contract changes from "OpenAPI primary, sample fallback" to "default baseline first, then merge OpenAPI (or sample fallback) on top." The guaranteed-field set is expanded beyond `name`/`namespace` to include `labels`, `annotations`, `metadata`, `spec`, `status`, and conflicting field types across sources are now rejected rather than silently overwritten.

## Impact

- Code: `internal/adapter/datasources/k8s/` — new `schema*.go` files, removed `inferrer.go`/`inferrer_test.go`; minor adjustments in `datasource.go` and `internal/adapter/sql/octosql/database.go`.
- Behavior: `SELECT *` and `DESCRIBE TABLE` now always surface the default top-level columns even on partial/empty clusters.
- No new external dependencies. Read-only; no write paths added.
