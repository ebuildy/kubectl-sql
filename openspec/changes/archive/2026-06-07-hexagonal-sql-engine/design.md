## Context

After `hexagonal-k8s-datasource`, client-go is confined to `internal/adapter/datasources/k8s` behind the `DataSource` port. octosql is still spread across three places: `cmd/root.go` (rewrite → parse → ParseNode → typecheck → optimize → ORDER BY/LIMIT → materialize → render setup, plus `rewriteQuery`/`typecheckNode`/`typecheckExpr`), `internal/executor` (`physical.Database`/`DatasourceImplementation`/execution `Node` over the k8s port), and `internal/output` (renders `octosql.Value` to table/json/csv).

This change applies the same port/adapter pattern (and boundary test) used for k8s and logging to the SQL engine. `internal/adapter/sql/octosql` already exists with a starter `functions.go` (custom `FunctionMap`).

## Goals / Non-Goals

**Goals:**
- An `Engine` port in `internal/port/sql` with no octosql types in any exported signature
- All octosql imports confined to `internal/adapter/sql/octosql` (+ nothing else), enforced by a boundary test
- `cmd` becomes octosql-free; it calls `engine.Execute(ctx, Query, w)`
- The engine consumes the k8s `DataSource` port (injected); the SQL adapter imports no client-go
- Zero behavior change — existing unit + integration suites pass unmodified

**Non-Goals:**
- Touching the k8s port/adapter (done in the previous change)
- Changing query semantics, the dot/arrow rewriter logic, output formats, or flags
- A second engine implementation (only octosql; the port just makes it swappable)
- Changing `SHOW TABLES` / `DESCRIBE TABLE`, which use the k8s port directly and never touched octosql

## Decisions

### 1. Port surface: `Engine.Execute` returns rendered output

`internal/port/sql/engine.go`:
```go
package sql

import (
    "context"
    "io"
)

// Query is a library-free description of a query to run.
type Query struct {
    SQL       string
    Output    string // "table" | "json" | "csv"
    Namespace string
    PageSize  int
    NoColor   bool
}

type Engine interface {
    // Execute runs the query and writes the rendered result to w.
    Execute(ctx context.Context, q Query, w io.Writer) error
}
```
No octosql types appear. Rendering lives inside the adapter, so the port stays a single method. `cmd` builds `Query` from flags and calls `Execute`.

**Alternative considered**: returning a neutral `RowSet` and rendering in `cmd`/`output`. Rejected per the chosen approach — keeping rendering in the adapter means `cmd` is fully octosql-free with a one-method port, and the existing `output` rendering (which is intrinsically about octosql values) moves wholesale rather than being rewritten against a neutral row model.

### 2. Adapter owns the whole pipeline; executor + output fold in

`internal/adapter/sql/octosql` absorbs:
- the pipeline from `cmd.runQueryWithWriter` (everything after `SHOW TABLES`/`DESCRIBE TABLE` routing): `rewriteQuery` + `rewriteDottedFields` + the regexes, `sqlparser.Parse`, `parser.ParseNode`, `typecheckNode`/`typecheckExpr`, `optimizer.Optimize`, ORDER BY/LIMIT typecheck+materialize, `physicalPlan.Materialize`, reverse-mapping of output field names
- `internal/executor` → the `physical.Database` (`kubernetesDatabase`), `DatasourceImplementation`, execution `Node`, and the `resolveFieldValue`/struct-building/`ResolveField` helpers
- `internal/output` → `Render` + table/json/csv writers + `valueToNativeTyped`/`valueToString`

The adapter keeps internal sub-files (e.g. `engine.go`, `pipeline.go`, `database.go`, `resolver.go`, `render.go`, `functions.go`) but one package `octosql`.

### 3. Engine constructed with the k8s DataSource injected

`func New(ds k8sport.DataSource) sql.Engine`. The engine builds its `physical.Database` (the former `executor.KubernetesDatabase`) over `ds`. `cmd` wires both: `ds := k8sadapter.New(...)`, `eng := octosqladapter.New(ds)`, `eng.Execute(...)`. The SQL adapter imports the k8s **port**, never the k8s adapter or client-go.

### 4. Logging stays via the logger port

The per-step debug logs currently in `cmd.runQueryWithWriter` ("query rewritten/parsed/typechecked/optimized/completed") move into the engine and continue to use `logger.FromContext(ctx)` (the logging port). "query accepted" / "cluster connection established" logs that are about wiring stay in `cmd`.

### 5. cmd after the change

`runQueryWithWriter` shrinks to: log "query accepted"; if `SHOW TABLES`/`DESCRIBE TABLE` → use k8s port (unchanged); else build `ds` + `eng` and call `eng.Execute(ctx, Query{...flags}, w)`. `runWatch` and the REPL `RunQuery` closure call the same `Execute`. The octosql-specific helpers and imports leave `cmd`.

### 6. Boundary enforced by test

A test (mirroring the k8s/logging boundary tests) walks the tree and fails if `github.com/cube2222/octosql` is imported outside `internal/adapter/sql/octosql`. The `test/` tree is excluded (e2e asserts on output, not octosql types).

## Risks / Trade-offs

- **Large move (executor + output + pipeline)** → stage tasks so the tree compiles after each: build port, build engine consuming existing executor/output, then fold executor/output in, then delete, then rewire cmd, then boundary test.
- **`output` is currently its own package with tests** → move `renderer_test.go` into the adapter and adapt; rendering behavior must stay identical (covered by e2e output assertions + JQ checks).
- **`functions.go` already present** → the starter custom `FunctionMap` is preserved and merged with octosql's base map inside the engine, exactly as `cmd` does today.
- **`os.Exit` ristretto note** → the comment in `cmd.Execute` about octosql ristretto goroutines stays in `cmd` (process lifecycle), even though octosql itself moves; the engine does not call `os.Exit`.
- **Two adapters, one query** → `cmd` is the composition root wiring k8s + sql adapters; both depend only on ports, never each other's libraries.

## Migration Plan

Structural refactor; no runtime migration. Rollback = revert. Stage order keeps `make lint build` and tests green at each step. Depends on `hexagonal-k8s-datasource` (already archived).

## Open Questions

- Should `Query` carry `Explain`/`DryRun` for the existing (currently unused) flags? Not in v1 — those flags are declared but not implemented today; keep `Query` to what `Execute` actually uses. Revisit if/when those flags are wired.
