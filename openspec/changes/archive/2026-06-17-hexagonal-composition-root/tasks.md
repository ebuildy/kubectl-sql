## 1. EngineFactory port and adapter

- [x] 1.1 Add `EngineFactory` interface (`New(cfg Config) Engine`) to `internal/port/sql/engine.go`
- [x] 1.2 Add `octosql.NewFactory(ds, sc)` in `internal/adapter/sql/octosql` returning an `EngineFactory` whose `New(cfg)` calls the existing `octosql.New(cfg, ds, sc)`
- [x] 1.3 Add an adapter test asserting `NewFactory(...).New(cfg)` returns a working `Engine` (executes a simple query)

## 2. Create the composition root (internal/app)

- [x] 2.1 Create package `internal/app`
- [x] 2.2 Add `app.NewQueryCommand(ctx, cfg)`: build `DataSource`, spellchecker, `EngineFactory`, and the csv-config `Mutator`; inject ports into the query command
- [x] 2.3 Add `app.NewReplCommand(ctx, cfg)`: build `DataSource`, `EngineFactory`, completion source, readline shell; wire the query runner and inject ports
- [x] 2.4 Add `app.NewUICommand(ctx, cfg, addr)`: build `DataSource`, `EngineFactory`, completion source, web server; inject ports

## 3. Convert domain command constructors to ports

- [x] 3.1 `query`: collapse `NewQueryCommand`/`NewQueryCommandWithDataSource` into one port-taking constructor; make `inREPL` an explicit option; drop `k8sAdapter`/`octosqlAdapter`/`spellcheckerAdapter` imports
- [x] 3.2 `query`: replace `octosqlAdapter.New(cfg, ...)` in `RunWithWriter` with `c.engines.New(cfg)`
- [x] 3.3 `query`: remove `newMutator()` and the inline csv-engine construction; use the injected `mut sqlPort.Mutator`; drop `mutatorAdapter`/`octosqlAdapter` imports from `delete.go`
- [x] 3.4 `repl`: take injected `DataSource`, `EngineFactory`, `ShellCompletionRunner`, and the shell; drop `k8sAdapter`/`shellAdapter`/`shellCompletionAdapter`/`octosqlAdapter` imports; remove the `@TODO: arch hexa` comment
- [x] 3.5 `ui`: take injected `DataSource`, `EngineFactory`, `ShellCompletionRunner`; replace inline engine construction in `RunJSON` with `EngineFactory.New(cfg)`; drop `k8sAdapter`/`octosqlAdapter`/`spellcheckerAdapter`/`shellCompletionAdapter`/`webAdapter` construction imports

## 4. Repoint cmd

- [x] 4.1 Change `cmd/root.go` to call `app.New*` builders instead of `commandQuery.New*`/`commandRepl.New*`/`commandUI.New*`
- [x] 4.2 Confirm `cmd` no longer imports any `internal/adapter/*` package for command wiring

## 5. Enforce and verify the boundary

- [x] 5.1 Add a boundary test asserting no package under `internal/domain/` imports any package under `internal/adapter/`
- [x] 5.2 Update existing query/repl/ui command tests to construct via the new port-taking constructors (inject fakes/real ports)
- [x] 5.3 Run `make lint build` and fix any issues
- [x] 5.4 Run `make test` (unit + envtest integration) and confirm it passes unchanged
