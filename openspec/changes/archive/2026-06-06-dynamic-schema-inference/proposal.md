## Why

`kubectl-sql` currently exposes only two columns for every resource: `name` and `namespace`. Users cannot discover or filter on real resource fields like `labels`, `annotations`, `status`, or `spec` without reading raw JSON manually.

## What Changes

- On `GetTable`, infer the schema via OpenAPI v3 (primary) or a 1-item LIST sample (fallback)
- Expose resource fields as typed octosql columns: `name`, `namespace`, plus every top-level key (`spec`, `status`, `metadata`, etc.)
- Nested maps become `TypeIDStruct` — accessible via `->` operator: `metadata->labels->app`
- Dot notation is rewritten to `->` automatically: `metadata.labels.app` → `metadata->labels->app`
- `SELECT *` returns all real resource fields
- `WHERE metadata->labels->app = 'nginx'` works natively
- Fall back to `name`/`namespace` only if no schema can be inferred
- Add `DESCRIBE TABLE <resource>` — lists column names and types for any resource

## Capabilities

### New Capabilities

- `dynamic-schema`: Schema inference via OpenAPI (primary) + sample (fallback) at `GetTable` time — exposes all resource fields as named struct-typed columns
- `describe-table`: `DESCRIBE TABLE <resource>` command listing all column names and types

### Modified Capabilities

- `k8s-datasource`: `GetTable` calls `SchemaInferrer`; row production builds `TypeIDStruct` values for nested maps

## Impact

- `internal/schema/` — new hexagonal package: `SchemaInferrer` port, `OpenAPIInferrer`, `SampleInferrer`, `CompositeInferrer`
- `internal/executor/executor.go` — accepts `SchemaInferrer`; `fieldToOctoType` builds `TypeIDStruct`; `resolveStructValue` recurses for nested structs
- `internal/output/renderer.go` — JSON renderer uses schema type info to render structs as named maps
- `cmd/root.go` — wires `CompositeInferrer`; dot-to-arrow rewriter; `DESCRIBE TABLE` handler
