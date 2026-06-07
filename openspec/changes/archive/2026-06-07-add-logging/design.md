## Context

`kubectl-sql` has no logging today; diagnostics are ad-hoc. AGENTS.md mandates "no global state — all dependencies injected via constructors or context." The query pipeline already threads a `context.Context` everywhere (k8s client, inferrers, executor, repl, watch), so context is the natural carrier for a logger. New dependencies must be justified in this design doc (guardrail #3).

## Goals / Non-Goals

**Goals:**
- A single leveled `*zap.Logger` per invocation, level set by `-v`/`-vv`
- Logger reachable from every pipeline component without global state
- Logs on stderr; stdout stays clean for piping
- Genuinely useful info/debug traces at execution boundaries

**Non-Goals:**
- Structured JSON log output / log files / rotation (v1 is human-readable console to stderr)
- Per-package log levels or named sub-loggers (one level for the whole run)
- Replacing the existing `fmt.Fprintf(os.Stderr, ...)` user-facing error messages — those are product output, not logs
- A `--log-format` flag

## Decisions

### 1. Hexagonal: `Logger` port and zap adapter in separate packages

**Decision**: A real ports-and-adapters split across two packages:

- **Port** — `internal/port/logger` — owns the `Logger` interface, the library-agnostic `Field` type and its constructors, `Options`, `Nop()`, and the context helpers. Imports no logging library. This is what every other package depends on.
- **Adapter** — `internal/adapter/logger/zap` — imports `go.uber.org/zap` and implements the port. It is the **only package in the repository that imports zap**.

Swapping zap means adding a sibling adapter package (e.g. `internal/adapter/logger/slog`) and changing one wiring line in `cmd`; no call site or port changes.

The port (`internal/port/logger/logger.go`):
```go
package logger

type Field struct { Key string; Value any }

type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    With(fields ...Field) Logger
    Sync() error
}

type Options struct { Verbosity int; NoColor bool }

// Field constructors so call sites never touch the underlying library:
func String(k, v string) Field { return Field{k, v} }
func Int(k string, v int) Field { return Field{k, v} }
func Err(err error) Field       { return Field{"error", err} }
func Any(k string, v any) Field { return Field{k, v} }

func Nop() Logger { /* no-op impl */ }
```

The adapter (`internal/adapter/logger/zap/zap.go`):
```go
package zap

import (
    "go.uber.org/zap"
    "github.com/ebuildy/kubectl-sql/internal/port/logger"
)

func New(opts logger.Options) logger.Logger { /* console encoder → stderr, level map, Field→zap.Field */ }
```

Level mapping and the stderr console encoder live in the adapter.

**Why a custom port over using `*zap.Logger` directly**: the requirement is that zap be replaceable without touching other code. A typed-field interface keeps call sites (`log.Info("query accepted", logger.String("resource", r))`) library-agnostic while still allowing structured fields. The `Field` indirection is the cost of decoupling.

**Library choice**: zap as requested, behind the adapter. Console encoder to stderr (not JSON production encoder) so `-v`/`-vv` output is terminal-readable. New deps: `go.uber.org/zap`, `go.uber.org/multierr` — confined to the adapter package, justified here per guardrail #3.

**Alternative considered**: exposing `*zap.Logger` everywhere. Rejected — leaks the library into every package, the exact coupling to avoid.

### 2. Verbosity flag: cobra `CountP("verbose", "v", 0, …)`

**Decision**: A count flag maps `-v`→1, `-vv`→2, `-vvv`→2+ naturally. Mapping: `0 → ErrorLevel`, `1 → InfoLevel`, `>=2 → DebugLevel`.

**Alternative considered**: a string `--log-level` flag. Rejected — the request specifies `-v`/`-vv` ergonomics; a count flag is the idiomatic cobra way and avoids parsing/validating level strings. `-v` is currently unused (`-w` watch, `-i` repl are the only short flags).

### 3. Logger plumbed via context (port-typed)

**Decision**: The **port** package owns the context helpers (port-typed, no library in any signature):
- `logger.IntoContext(ctx, Logger) context.Context`
- `logger.FromContext(ctx) Logger` — returns the stored logger or `Nop()` if absent

The **adapter** owns construction: `zap.New(logger.Options{...}) logger.Logger`.

Wiring lives in `cmd`'s root `PersistentPreRunE` — the composition root — which is the single place that imports the concrete adapter: it calls `zap.New(...)` and stores the result via `cmd.SetContext(logger.IntoContext(cmd.Context(), l))`. Every component receives `ctx` and calls `logger.FromContext(ctx)` against the port only. This satisfies "no global state" and the nop fallback means tests and missing-context paths never panic.

Note: `cmd` (the composition root) is allowed to import the adapter to wire it — that is the one legitimate adapter import outside the adapter package itself. The boundary test (Decision 1 / task) accounts for this by allowing the adapter import in `cmd` wiring as well as the adapter package.

**Alternative considered**: a package-level global logger. Rejected — violates the no-global-state guardrail.

### 4. stderr, console encoder (inside the adapter)

**Decision**: The adapter writes to stderr (`zapcore.Lock(os.Stderr)`) with a console encoder; stdout is reserved for query results. `--no-color` disables level coloring. All of this is encapsulated in `internal/adapter/logger/zap` — callers only know "logs go somewhere at a level."

## Risks / Trade-offs

- **Log noise at `-vv` on large clusters** (per-page LIST debug lines) → keep pagination logs concise (page number + count), not full payloads.
- **zap deps in a "lean dependency footprint" project** → only two small, widely-used modules, confined to the adapter package; documented here per guardrail #3.
- **Logger constructed before flags parsed** → use `PersistentPreRunE`, which runs after flag parsing, so verbosity is known.
- **Field indirection (`logger.Field` vs `zap.Field`)** → the decoupling cost. Provide enough constructors (`String`, `Int`, `Err`, `Any`) that call sites are ergonomic; the adapter maps `logger.Field` → `zap.Field` once.
- **Leak risk** → zap could accidentally get imported elsewhere. Guard with a test that scans the tree: only `internal/adapter/logger/zap` (and `cmd` wiring) may import `go.uber.org/zap` (task 2.5). This keeps the hexagonal boundary enforced, not just documented.

## Migration Plan

Additive only. No flag removed, no output on stdout changed, exit codes unchanged. Rollback = revert the change; nothing persisted.

### 5. Duration timing in debug logs

**Decision**: The port provides a `Duration(key string, d time.Duration) Field` constructor that records elapsed time in milliseconds under `<key>_ms`. Key boundaries log their elapsed time at debug level: total query (`runQueryWithWriter`), schema inference (`CompositeInferrer`), and each LIST page (executor). This keeps timing library-agnostic (no zap-specific duration encoding leaks across the port).

## Open Questions

- Should `-vvv` unlock zap's caller/stacktrace annotation? Deferred — `>=2` maps to debug for v1; finer tiers can come later without breaking the flag.
