## Why

`kubectl-sql` currently has no structured logging — debugging a failed query or slow cluster call means adding `fmt.Println` by hand. A leveled logger lets users opt into diagnostics (`-v`, `-vv`) without polluting normal output, and gives the code a single, consistent place to emit debug/info traces.

## What Changes

- Add `go.uber.org/zap` as the logging library, hidden behind a hexagonal port/adapter boundary
- Introduce a port package `internal/port/logger` — a domain-owned `Logger` interface (+ `Field`, constructors, `Options`, `Nop`, context helpers) that all code calls
- Introduce an adapter package `internal/adapter/logger/zap` — the only package that imports zap; implements the port. Swapping zap means adding a sibling adapter, not touching call sites
- Expose the logger via context (`IntoContext`/`FromContext`, nop fallback)
- Add a repeatable `-v` / `--verbose` count flag controlling verbosity:
  - default (no flag): `error` level
  - `-v`: `info` level
  - `-vv` (or more): `debug` level
- Emit useful logs at key boundaries: cluster connection, schema inference, query parse/typecheck/execute, pagination, REPL/watch lifecycle
- Logs go to **stderr** so they never corrupt query results on stdout (table/json/csv stay clean and pipeable)

## Capabilities

### New Capabilities

- `logging`: Leveled logging via zap, a `-v`/`-vv` verbosity flag, stderr output, and debug/info traces at key execution boundaries

### Modified Capabilities

_None — logging is additive. Existing query output on stdout and exit codes are unchanged._

## Impact

- `go.mod`: add `go.uber.org/zap` (and its dep `go.uber.org/multierr`) — imported only by the adapter package
- New port package `internal/port/logger/` and adapter package `internal/adapter/logger/zap/`
- `cmd/root.go`: register `-v`/`--verbose` count flag; wire `zap.New` in `PersistentPreRunE` and attach the port logger to the command context (the one place `cmd` imports the adapter)
- `internal/k8s`, `internal/schema`, `internal/executor`, `internal/repl`, and the query pipeline in `cmd/`: add debug/info log calls via `logger.FromContext(ctx)` (port only)
- README + flag table: document `-v` / `-vv`
