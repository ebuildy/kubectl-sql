# ADR-002: SQL Engine — octosql

**Date:** 2026-06-06  
**Status:** Accepted  
**Deciders:** Thomas Decaux

---

## Context

`kubectl-sql` needs a SQL execution engine that can accept a query string, evaluate it against a custom datasource, and return typed results. The options are:

1. **Write a SQL engine from scratch** — lexer, parser, AST, planner, executor
2. **Embed an existing Go SQL library** — plug in Kubernetes as a datasource
3. **Use a database driver shim** — expose Kubernetes via a `database/sql` driver and use standard SQL

---

## Decision

Use **[octosql](https://github.com/cube2222/octosql) v0.13** as the embedded SQL engine.

---

## Rationale

### Why octosql

| Concern | Detail |
|---|---|
| **Custom datasource API** | octosql exposes `physical.Database` and `physical.DatasourceImplementation` interfaces. Implementing them is the only integration point — no SQL-to-REST translation layer needed. |
| **Streaming semantics** | octosql is built around a streaming execution model with watermarks. This maps naturally to Kubernetes LIST pagination: results stream through the engine as pages arrive, and a watermark at end-of-stream triggers aggregate flush (`COUNT(*)`, `GROUP BY`). |
| **Struct types** | `octosql.TypeIDStruct` lets us expose nested Kubernetes objects (e.g. `metadata`, `status`) as typed structs with native `->` field access, rather than serializing them to opaque JSON strings. |
| **Full SQL subset out of the box** | `SELECT`, `WHERE`, `ORDER BY`, `LIMIT`, `GROUP BY`, `DISTINCT`, aggregates, `LIKE`, `IN`, subqueries — all implemented and tested by octosql. Writing equivalent functionality from scratch would take months. |
| **Pure Go, no CGo** | octosql has no native dependencies. The binary stays a single static Go executable — essential for a `kubectl` plugin. |
| **Query optimizer** | `optimizer.Optimize()` applies filter pushdown, dead field elimination, and filter merging automatically. Unused columns are pruned before `Materialize()` is called on the datasource. |

### Why not the alternatives

**Write from scratch**: The SQL subset required (aggregates, structs, streaming watermarks, query optimization) would require a multi-month effort with significant ongoing maintenance. The risk of subtle correctness bugs in a hand-rolled parser and executor is high.

**`database/sql` driver shim**: The `database/sql` interface is row-oriented and synchronous, with no concept of streaming, watermarks, or struct types. It would require buffering all results in memory before returning them — defeating the pagination design — and would not support typed nested objects.

**DuckDB** (`go-duckdb`): DuckDB is an excellent analytical SQL engine with a rich standard SQL dialect and strong aggregate performance. However it is ruled out for two reasons: (1) it requires CGo, which breaks the single static binary requirement for a `kubectl` plugin, and (2) its datasource extension API is C-level, making it far more complex to expose a live Kubernetes LIST stream as a virtual table compared to octosql's Go interface. DuckDB would be a strong candidate if the CGo constraint were lifted and the datasource model were purely in-memory (e.g. a future mode that materialises the full cluster state locally for complex analytical queries).

**Other embedded SQL engines** (e.g. `xo/xsql`, `rqlite/go-sqlite3`): These are either SQLite wrappers (CGo), query builders (not engines), or lack a custom datasource plugin API.

---

## Consequences

### Accepted trade-offs

- **`ristretto` goroutine leak**: octosql's `functions.FunctionMap()` initialises ristretto caches that spawn background goroutines with no shutdown path. The process is terminated with `os.Exit(0)` in `main.go` to force-kill them after results are printed. This is intentional and documented.

- **SQL dialect is octosql's, not standard SQL**: The `->` struct field access operator, watermark-based aggregate flushing, and table qualifier syntax (`k8s.pods`) are octosql-specific. Queries are not portable to other SQL engines. This is acceptable because `kubectl-sql` is a purpose-built CLI tool, not an interoperability layer.

- **`FROM` clause requires `k8s.` prefix internally**: The SQL rewriter in `cmd/root.go` adds the `k8s.` prefix to bare table names before parsing so octosql routes them to the `KubernetesDatabase`. This rewriting is transparent to the user.

- **Vendored source copy in `external/octosql/`**: A reference copy of the octosql source is kept for in-session context (AI-assisted development). It is not imported — `go.mod` imports from the published module.

---

## Future

If octosql v0.13 is ever abandoned or incompatible with a future Go version, the `physical.Database` + `physical.DatasourceImplementation` interfaces are narrow enough that a drop-in replacement engine could be adapted with changes confined to `cmd/root.go` and `internal/executor/executor.go`.
