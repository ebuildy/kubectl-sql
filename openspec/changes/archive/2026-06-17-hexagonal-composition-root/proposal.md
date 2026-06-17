## Why

The domain layer reaches down and constructs concrete adapters: `NewQueryCommand`, `NewReplCommand`, and `NewUICommand` all call `k8sAdapter.New(...)`, and the query/ui commands build the octosql engine, mutator, spellchecker, and completion source inline. This inverts the hexagonal dependency rule — `internal/domain/...` imports `internal/adapter/...` — making the domain hard to test in isolation and coupling it to specific libraries. A leftover `// @TODO: arch hexa should be moved to main` in `repl/command.go` already flags this.

## What Changes

- Introduce a new composition-root package `internal/app` that owns every `adapter.New(...)` call and injects ports into the domain command constructors.
- Add a single new port `sql.EngineFactory` (`internal/port/sql`) so the SQL engine can be created per call with a domain-chosen `Config` without importing the octosql adapter. The octosql adapter gains `NewFactory(ds, sc)` implementing it.
- Change the domain command constructors (`query`, `repl`, `ui`) to accept ports only (`DataSource`, `EngineFactory`, `Mutator`, `ShellCompletionRunner`). They stop importing any `internal/adapter/*` package.
- Move construction of the driving adapters (readline shell, web server) into `internal/app`, resolving the `repl/command.go` TODO.
- Shrink `cmd/root.go` to flag parsing plus calls into `internal/app`.
- No behavioral, grammar, output, or flag change — this is a pure internal hexagonal refactor; all existing tests pass unchanged.

## Capabilities

### New Capabilities
- `composition-root`: All concrete adapter construction and dependency wiring live in `internal/app`; the domain (`internal/domain/...`) depends only on ports and never imports `internal/adapter/...`. `cmd` is reduced to flag parsing plus calls into `internal/app`.

### Modified Capabilities
- `sql-engine-port`: Add an `EngineFactory` port so consumers obtain an `Engine` for a given `Config` without importing the octosql adapter; engine construction by consumers goes through the factory rather than `octosql.New`.

## Impact

- New package: `internal/app`.
- New port type: `sql.EngineFactory` in `internal/port/sql`; new `octosql.NewFactory` constructor in `internal/adapter/sql/octosql`.
- Modified: `internal/domain/commands/query`, `internal/domain/commands/repl`, `internal/domain/commands/ui` (constructors take ports; drop adapter imports; `newMutator`/`NewQueryCommandWithDataSource` collapse into injection).
- Modified: `cmd/root.go` (delegates wiring to `internal/app`).
- No new third-party dependencies. No public CLI surface change.
