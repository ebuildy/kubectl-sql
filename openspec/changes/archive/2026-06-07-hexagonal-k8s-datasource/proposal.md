## Why

Kubernetes access (`k8s.io/client-go`, `apimachinery`, discovery, OpenAPI) is currently spread across `internal/k8s`, all of `internal/schema`, `internal/executor`, and `cmd/root.go`. That couples the query engine and the CLI directly to client-go and forces every consumer to speak in `GroupVersionResource` and `unstructured` terms. To make the data source swappable and the boundary explicit, the cluster integration should sit behind a port, with all client-go code confined to one adapter package.

This is the **first of two** hexagonal refactors. This change extracts the Kubernetes data-source port + adapter. A follow-up change extracts the SQL-engine (octosql) port + adapter, which will consume this k8s port.

## What Changes

- Introduce a port package `internal/port/datasources/k8s` defining a `DataSource` interface in plain Go / domain terms (no `client-go`, no `apimachinery` types in any exported signature):
  - resolve a table name (plural/short/kind) to a canonical resource identity
  - list a resource's objects (paginated) as `[]map[string]any`
  - infer a resource's schema as `[]schema.Field`
  - list all queryable resources (for `SHOW TABLES` / completion)
- Introduce an adapter package `internal/adapter/datasources/k8s` — the **only** package importing `k8s.io/client-go`, `k8s.io/apimachinery`, `k8s.io/kube-openapi`, or `k8s.io/client-go/discovery`. It implements the port over the existing dynamic client, REST mapper, discovery, OpenAPI + sample inferrers.
- Move the schema inference adapters (OpenAPI/sample/composite inferrers, which import apimachinery) into the k8s adapter; keep the library-agnostic `schema.Field`/`FieldType` domain types in a port-side package.
- Rewire `cmd/root.go` so the query pipeline, `SHOW TABLES`, `DESCRIBE TABLE`, REPL completion, and watch obtain data through the port, not client-go.
- A boundary test asserts client-go/apimachinery/discovery imports appear only inside `internal/adapter/datasources/k8s` (and the `cmd` composition root that wires it).

Behavior (query output, exit codes, flags) is unchanged — this is a structural refactor.

## Capabilities

### New Capabilities

- `k8s-datasource-port`: A Kubernetes data-source port (domain-typed interface) and its client-go adapter, with the library confined to the adapter package

### Modified Capabilities

_None — no requirement-level behavior changes. `sql-execution`, `dynamic-schema`, `show-tables`, `describe-table`, and `k8s-datasource` keep their observable contracts; only the internal wiring moves._

## Impact

- New packages: `internal/port/datasources/k8s/` (interface + domain types), `internal/adapter/datasources/k8s/` (client-go implementation, incl. relocated inferrers)
- Removed/emptied: `internal/k8s/`, the apimachinery-importing files in `internal/schema/` (moved to the adapter)
- `cmd/root.go`, `internal/repl/complete.go`: depend on the port instead of client-go
- The `octosql` executor (`internal/executor`) is rewired in this change to obtain rows/schema/resolution through the k8s port (so client-go leaves it); change B later relocates the executor itself into the octosql adapter behind the sql port
- `go.mod`: no new deps; existing k8s deps become adapter-only
