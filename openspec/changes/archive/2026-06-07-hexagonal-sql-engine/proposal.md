## Why

octosql is currently spread across `cmd/root.go` (the entire parse → typecheck → optimize → materialize pipeline), `internal/executor` (implements octosql's `physical.Database`), and `internal/output` (renders `octosql.Value`s). That couples the CLI and rendering directly to octosql. This change — the second of the two hexagonal refactors, mirroring the just-shipped `hexagonal-k8s-datasource` — puts the SQL engine behind a port so octosql lives in exactly one adapter package and can be swapped without touching `cmd`.

## What Changes

- Introduce a port package `internal/port/sql` defining an `Engine` interface in plain Go / domain terms (no `github.com/cube2222/octosql/*` types in any exported signature):
  - `Execute(ctx, Query, w io.Writer) error` — runs a SQL string and writes the rendered result (table/json/csv) to the writer
  - `Query` carries the SQL string plus options (output format, namespace, page size, no-color)
- Introduce an adapter package `internal/adapter/sql/octosql` — the **only** package importing `github.com/cube2222/octosql/*`. It owns the full pipeline: dot/arrow query rewrite, parse, logical/physical plan, typecheck, optimize, materialize, ORDER BY / LIMIT handling, execution, and rendering of result rows to table/json/csv.
- Fold `internal/executor` (the `physical.Database` implementation over the k8s port) and `internal/output` (octosql value rendering) into the octosql adapter.
- The engine is constructed with the k8s `DataSource` port injected: `octosql.New(ds)`. The adapter consumes the k8s port to obtain rows/schema; it does not import client-go.
- Rewire `cmd/root.go`: build the k8s adapter, build the octosql engine with it, and call `engine.Execute(...)` for queries (and in the REPL/watch paths). `SHOW TABLES` and `DESCRIBE TABLE` continue to use the k8s port directly (they never used octosql). `cmd` no longer imports octosql.
- A boundary test asserts `github.com/cube2222/octosql` is imported only inside `internal/adapter/sql/octosql`.

Behavior (query results, output formats, exit codes, flags, logging) is unchanged — this is a structural refactor.

## Capabilities

### New Capabilities

- `sql-engine-port`: A SQL-engine port (domain-typed `Engine` interface) and its octosql adapter, with octosql confined to the adapter package

### Modified Capabilities

_None — no requirement-level behavior changes. `sql-execution`, `output-renderer`, `k8s-datasource`, `show-tables`, `describe-table`, `watch-mode`, and `sql-repl` keep their observable contracts; only internal wiring moves._

## Impact

- New packages: `internal/port/sql/` (interface + `Query` type), `internal/adapter/sql/octosql/` (octosql pipeline + rendering; already started with `functions.go`)
- Removed/folded: `internal/executor/` and `internal/output/` move into the octosql adapter
- `cmd/root.go`: depends on `internal/port/sql` + the adapter constructor; drops all octosql imports and the `rewriteQuery`/`typecheckNode`/`typecheckExpr` helpers (they move to the adapter)
- `go.mod`: no new deps; existing octosql dep becomes adapter-only
- Builds on `hexagonal-k8s-datasource`: the octosql engine consumes the k8s `DataSource` port
