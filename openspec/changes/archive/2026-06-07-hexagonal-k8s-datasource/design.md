## Context

Today client-go/apimachinery/discovery/openapi are imported by `internal/k8s`, all of `internal/schema`, `internal/executor`, and `cmd/root.go`. The `executor.KubernetesDatabase` both implements octosql's `physical.Database` AND drives client-go LIST/discovery — two libraries entangled in one type. AGENTS.md mandates dependency injection and warns the SQL/k8s layers are a public seam.

This change is the first of two (see proposal). It establishes the **k8s data-source port** and moves all client-go behind one adapter. The follow-up change establishes the **sql port** (octosql adapter) which will consume this k8s port. We follow the same port/adapter pattern already shipped for logging (`internal/port/logger` + `internal/adapter/logger/zap`), including a boundary test.

## Goals / Non-Goals

**Goals:**
- A `DataSource` port in `internal/port/datasources/k8s` with no `k8s.io/*` types in any exported signature
- All client-go/apimachinery/discovery/openapi confined to `internal/adapter/datasources/k8s` (+ `cmd` wiring), enforced by a boundary test
- `cmd`, `repl`, and the executor obtain cluster data through the port
- Zero behavior change — existing unit + integration suites pass unmodified

**Non-Goals:**
- Touching octosql or `internal/output` (that is change B — the sql port)
- Changing the `schema.Field` model, the dot/arrow rewriter, or any query semantics
- New CLI flags or output changes
- A generic multi-datasource abstraction (only k8s for now; the port just makes it swappable)

## Decisions

### 1. Port surface: domain-typed `DataSource`

`internal/port/datasources/k8s/datasource.go`:
```go
package k8s

import (
    "context"
    "github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// Resource is a canonical, library-free identity for a queryable kind.
type Resource struct {
    Name       string   // canonical plural, e.g. "pods"
    Namespaced bool
    Aliases    []string // short names, e.g. "po"
    Group      string
    Version    string
}

type DataSource interface {
    // Resolve maps a user-typed table name (plural/short/kind) to a Resource.
    Resolve(ctx context.Context, table string) (Resource, error)
    // Resources enumerates all queryable resources (for SHOW TABLES / completion).
    Resources(ctx context.Context) ([]Resource, error)
    // InferSchema returns the column model for a resource.
    InferSchema(ctx context.Context, r Resource) ([]schema.Field, error)
    // List returns objects for a resource as plain maps, honoring namespace + page size.
    // pageFn is called once per page so callers can stream without buffering the cluster.
    List(ctx context.Context, r Resource, opts ListOptions, pageFn func(page []map[string]any) error) error
}

type ListOptions struct {
    Namespace string
    PageSize  int64
}
```
No `client-go`, `apimachinery`, `unstructured`, or `GroupVersionResource` appears here. `schema.Field` is library-free already.

**Alternative considered**: returning a fully buffered `[][]map[string]any`. Rejected — the executor streams pages via octosql's `produce`; a `pageFn` callback preserves streaming without leaking octosql into the port.

### 2. Move `schema.Field` domain types to a port-side package

The `schema.Field`/`FieldType` types are library-free and are referenced by the port. To avoid an import cycle (port → schema → ... ) and keep the port self-contained, relocate the **pure types** (`Field`, `FieldType`, the ignored-field set) into `internal/port/schema`. The **inferrer implementations** (OpenAPI/sample/composite — which import apimachinery) move into the k8s adapter. `internal/schema/walk.go` (pure, no k8s) moves to the port-side package or the adapter depending on whether it touches apimachinery (it does not → port-side helper, or keep as an internal helper used by the adapter).

**Alternative considered**: leaving types in `internal/schema`. Rejected — `internal/schema` currently mixes pure types with apimachinery-importing inferrers; splitting is required to keep the port library-free.

### 3. Adapter: one package implements the port

`internal/adapter/datasources/k8s` absorbs:
- `internal/k8s/client.go` (dynamic client, REST mapper, discovery bootstrap)
- `internal/schema/openapi_inferrer.go`, `sample_inferrer.go`, `composite_inferrer.go`
- the LIST/pagination logic currently in `executor.kubernetesExecution.Run` (the part that talks to client-go)

It maps client-go types → domain types at the boundary: `unstructured.Unstructured.Object` → `map[string]any`, `GroupVersionResource` ↔ `Resource`, discovery results → `[]Resource`.

A constructor `New(kubeconfig, kubeContext string) (k8sport.DataSource, error)` is the single wiring entry point.

### 4. Executor consumes the port (interim, for this change)

`internal/executor` still implements octosql's `physical.Database` in this change, but instead of holding a `dynamic.Interface` + mapper + inferrer it holds a `k8sport.DataSource`. Its `Run` calls `ds.List(..., pageFn)` and maps each page's `map[string]any` rows to octosql values (the existing `resolveFieldValue` logic, unchanged). This removes client-go from `internal/executor` now; change B will relocate this glue into the octosql adapter.

### 5. Boundary enforced by test

A test (mirroring the logging `boundary_test.go`) walks the tree and fails if `k8s.io/client-go`, `k8s.io/apimachinery`, `k8s.io/kube-openapi`, or `k8s.io/client-go/discovery` is imported outside `internal/adapter/datasources/k8s` and `cmd`.

## Risks / Trade-offs

- **Large diff across many files** → staged tasks that keep the tree compiling after each stage; rely on the existing integration suite as the regression gate.
- **Import cycles** when moving `schema.Field` → resolve by putting pure types in `internal/port/schema`, depended on by both port and adapter.
- **`cmd` legitimately imports the adapter** (composition root) → the boundary test allowlists `cmd`, same as the logging boundary test.
- **`DESCRIBE TABLE` / `SHOW TABLES` currently call discovery/mapper directly in `cmd`** → these move behind `Resolve`/`Resources`/`InferSchema`; output must stay byte-identical (covered by existing e2e scenarios).
- **Executor still octosql-coupled after this change** → acceptable; that is explicitly change B's scope. This change is independently shippable and leaves a compiling, passing tree.

## Migration Plan

Structural refactor; no runtime migration. Rollback = revert. Stage order in tasks keeps `make lint build` and tests green at each step. The follow-up sql-port change depends on this one.

## Open Questions

- Should `Resource` carry enough to render the `SHOW TABLES` columns (GROUP/VERSION/ALIASES) directly? Yes — included in the struct so `cmd` formats without re-querying discovery. Confirmed in the port shape above.
