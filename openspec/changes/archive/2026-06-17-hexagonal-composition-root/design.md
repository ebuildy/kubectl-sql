## Context

`kubectl-sql` follows a ports-and-adapters layout: `internal/port/*` holds interfaces, `internal/adapter/*` holds concrete implementations, and `internal/domain/commands/*` holds the application use cases (query, repl, ui). The dependency rule should point inward — domain depends on ports, the composition root depends on everything — but today the domain constructors build adapters directly:

- `query.NewQueryCommand` → `k8sAdapter.New(...)`; `RunWithWriter` → `octosqlAdapter.New(...)`; `delete.newMutator` → `mutatorAdapter.New(octosqlAdapter.New(...), ds)`; `spellcheckerAdapter.New()` inline.
- `repl.NewReplCommand` → `k8sAdapter.New(...)`, builds the readline shell and completion source in `Run`.
- `ui.NewUICommand` → `k8sAdapter.New(...)`, builds the completion source and the octosql engine per call.

`cmd/root.go` parses cobra flags into `api.Config` and calls these constructors. There is already a clean hint: `query.NewQueryCommandWithDataSource(config, ds)` takes the `DataSource` port, and the `mut sqlPort.Mutator` field exists for test injection.

The dependencies divide into three timing classes:

| Dependency | Config known at | Strategy |
|---|---|---|
| `k8s.DataSource` | wiring time (`api.Config`) | build once, inject |
| `Mutator` (csv engine + ds) | wiring time (fixed `"csv"` policy) | build once, inject |
| completion source, readline shell, web server | wiring time | build in `internal/app` |
| SQL `Engine` | run time (output mode varies: user `--output` / `"json"` / `"csv"`) | inject a factory |

## Goals / Non-Goals

**Goals:**
- The domain (`internal/domain/...`) imports no `internal/adapter/...` package.
- All adapter construction and wiring lives in one place: `internal/app`.
- `cmd/root.go` is reduced to flag parsing plus calls into `internal/app`.
- Resolve the `// @TODO: arch hexa should be moved to main` in `repl/command.go`.
- Zero observable behavior change; existing unit and integration suites pass unchanged.

**Non-Goals:**
- No new SQL grammar, output format, CLI flag, or exit-code change.
- No change to the `Engine`, `Mutator`, `DataSource`, or `ShellCompletionRunner` method sets (only a new `EngineFactory` port is added alongside).
- No swap of the octosql, readline, or web libraries.
- Not reworking the internals of any adapter — only where they are constructed.

## Decisions

### Decision 1: Introduce `internal/app` as the composition root

`cmd` → `internal/app` → {`internal/adapter/*`, `internal/domain/*`}. `internal/app` exposes builders such as `app.NewQueryCommand(ctx, cfg)`, `app.NewReplCommand(ctx, cfg)`, `app.NewUICommand(ctx, cfg, addr)` that construct adapters once and inject ports into the domain constructors. `cmd/root.go` keeps cobra flag parsing and `api.Config` assembly, then delegates.

**Alternative considered:** keep wiring in `cmd`. Rejected because `cmd` is the cobra/CLI adapter; mixing dependency wiring into it keeps two responsibilities tangled and was the user-requested separation. A dedicated package keeps the cobra layer thin and gives wiring a single testable home.

### Decision 2: Add an `EngineFactory` port instead of injecting prebuilt engines

The SQL engine is built per call with a `Config` chosen by the domain (query uses the user's `--output`; ui uses `"json"`; the mutator's resolving SELECT uses `"csv"`). To remove the octosql import from the domain while letting it keep choosing the `Config`, add:

```go
// internal/port/sql
type EngineFactory interface {
    New(cfg Config) Engine
}
```

The octosql adapter implements it: `octosql.NewFactory(ds, sc)` returns a factory whose `New(cfg)` calls the existing `octosql.New(cfg, ds, sc)`. Domain code calls `c.engines.New(cfg)`.

**Alternative considered:** prebuild each concrete engine (one per output mode) in `internal/app` and inject them as `sql.Engine` values. Rejected because the domain would then carry several engine fields and the "how many engines and why" policy would leak into wiring. The factory is one injected dependency and keeps `Config` policy domain-side.

### Decision 3: Build the mutator in `internal/app`, collapse `newMutator`

The mutator's resolving SELECT uses a fixed `"csv"` config whose namespace/page-size come from `api.Config` — all known at wiring time. So `internal/app` builds `mutatorAdapter.New(engineFactory.New(csvCfg), ds)` once and injects it. The existing `mut sqlPort.Mutator` field becomes the single source (production injects the real one; tests inject a fake), so `query.newMutator()` and the per-call csv-engine construction are removed.

### Decision 4: Collapse the dual query constructors

`NewQueryCommand` and `NewQueryCommandWithDataSource` merge into one constructor that takes ports. The `inREPL` distinction (which suppresses the DELETE progress bar and selects the REPL input reader) becomes an explicit option/parameter set by the REPL builder in `internal/app` rather than being implied by which constructor was called.

### Decision 5: Driving adapters constructed in `internal/app`

The readline shell and the web server are driving (primary) adapters — they call into the application. `internal/app` constructs them and wires them to the domain command (the query runner), so the domain no longer imports `shellAdapter`/`shellCompletionAdapter`/`webAdapter`. The completion source (`ShellCompletionRunner` port) is built in `internal/app` and injected into the repl/ui commands.

## Risks / Trade-offs

- [Hidden behavior coupling to construction order] → Mitigation: the refactor is import-only/move-only; rely on the existing unit + envtest integration suites passing byte-for-byte unchanged as the acceptance gate (`make lint test`).
- [`inREPL`/progress-bar and confirmation-reader behavior regress when the dual constructor collapses] → Mitigation: cover the REPL vs one-shot DELETE paths with the existing tests; keep the option explicit and defaulted to the one-shot behavior.
- [Import cycle between `internal/app` and domain] → Mitigation: `internal/app` imports domain and adapters; domain imports neither, so the graph stays acyclic by construction. Add a boundary test asserting `internal/domain/...` imports no `internal/adapter/...`.
- [Scope creep into adapter internals] → Mitigation: this change only moves construction sites and adds the factory; any adapter-internal change is out of scope and deferred.

## Migration Plan

Single-PR refactor (no runtime migration): add the `EngineFactory` port + `octosql.NewFactory`; create `internal/app`; switch domain constructors to ports; repoint `cmd/root.go`; delete the now-dead inline construction. Rollback is reverting the PR. Acceptance: `make lint test` green and the boundary import test passing.

## Open Questions

None.
