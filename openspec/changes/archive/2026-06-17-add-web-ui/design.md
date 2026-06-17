## Context

`kubectl-sql` is a hexagonal Go application. The terminal entry points (`cmd/root.go`) wire
**domain commands** (`internal/domain/commands/{query,repl}`) which depend on **ports**
(`internal/port/{sql,datasources/k8s,autocomplete,api}`) implemented by **adapters**
(`internal/adapter/{sql/octosql,datasources/k8s,shell/completion,...}`).

The reusable building blocks for a web UI already exist:

- `k8sAdapter.New(ctx, kubeconfig, context, namespace)` → a `k8sPort.DataSource`.
- `octosqlAdapter.New(sqlPort.Config{Output:"json", ...}, ds, spellcheckerAdapter.New())` →
  a `sqlPort.Engine` whose `Execute(ctx, sqlPort.Query{SQL}, w)` writes rendered output (JSON
  when `Output:"json"`) to an `io.Writer`. It returns `*sqlPort.SuggestionError` for single-token
  typos (`Message()` / `CorrectedSQL`).
- `shellCompletionAdapter.NewShellCompletion(ctx, ds, octosqlAdapter.FunctionNames())` → an
  `autocomplete.ShellCompletionRunner` with `Do(line []rune, pos int) ([][]rune, int)` and
  `Prefetch(query string)`.
- `query.isDeleteStatement(query)` already classifies mutating statements (used to gate DELETE).

The web server is therefore a **primary/driving adapter**: it accepts HTTP requests and drives
the same ports the CLI drives. No existing port behavior changes.

## Goals / Non-Goals

**Goals:**
- `kubectl sql --ui` starts a local server, serves one embedded HTML page + a small JSON API,
  and shuts down cleanly on Ctrl-C.
- Reuse the existing SQL engine (JSON mode) and completion source verbatim — no query/parse
  logic duplicated in the web layer.
- Keep the front-end tiny: vanilla JS + small CSS, `go:embed`-ed, no framework or build step.
- Keep the browser surface read-only: reject DELETE/mutations at the API.

**Non-Goals:**
- No authentication / multi-user / remote exposure hardening (default bind is loopback).
- No DELETE or any write path from the browser (CLI-only, unchanged).
- No persistence of query history, no saved queries, no multi-tab session state.
- No rich editor library (CodeMirror/Monaco) — highlighting is a lightweight overlay.

## Decisions

### 1. HTTP server is a driving adapter, wired by a thin domain command

`internal/adapter/web` holds the `net/http` server, handlers, and embedded assets. A new
`internal/domain/commands/ui` (`UICommand`) wires the data source, engine factory, and completion
source into the web adapter and owns the server lifecycle — mirroring how `ReplCommand` wires the
readline adapter. `cmd/root.go` constructs `UICommand` when `--ui` is set, before the
query/REPL branch.

The adapter depends on small **driving ports** it defines for what it needs, rather than importing
domain/adapters directly:
- a `QueryRunner` with `RunJSON(ctx, sql string) (QueryResult, error)`, and
- a `Completer` with `Complete(line string, pos int) []string`.

The `ui` command implements these by delegating to `octosqlAdapter` (JSON engine) and the
completion source. This keeps the HTTP adapter testable with fakes and free of k8s/octosql
imports.

*Alternative considered:* let handlers import `octosqlAdapter`/`k8sAdapter` directly. Rejected —
it would couple the primary adapter to secondary adapters and make handler tests require a
cluster.

### 2. Query path reuses the JSON renderer, then re-shapes to `{columns, rows}`

`POST /api/query` builds an engine with `sqlPort.Config{Output:"json"}` and `Execute`s into a
buffer. The existing JSON output is the source of truth for values/formatting. The handler then
adapts that into the documented `{ "columns": [...], "rows": [...] }` response. Column order is
derived from the query's SELECT projection / first row key order as the renderer emits it.

*Alternative considered:* add a new structured (non-rendered) engine method returning typed rows.
Rejected for v1 — reusing the JSON renderer avoids touching the engine port and guarantees the UI
shows exactly what `--output json` shows.

### 3. Completion endpoint wraps `ShellCompletionRunner.Do`

`GET /api/complete?line=<...>&pos=<n>` calls `Do([]rune(line), pos)` and returns the candidate
suffixes joined onto the shared prefix so the client gets full tokens to insert. `Prefetch` is
called best-effort to warm the column cache. Using GET keeps it cache-friendly and trivially
called from `fetch`; the line is passed as a query parameter (URL-encoded).

### 4. Front-end: textarea + highlight overlay + fetch

The editor is a `<textarea>` layered over a `<pre>` highlight layer (the classic
"transparent textarea over highlighted pre" technique) — gives syntax coloring with zero editor
dependencies. A regex tokenizer colors keywords, strings, and `->`. Autocomplete calls
`/api/complete` (debounced) and renders a small candidate list; Enter/Tab inserts. Submit
(Ctrl/Cmd+Enter or a Run button) POSTs to `/api/query` and renders the JSON into a `<table>`.
All assets live under `internal/adapter/web/assets/` and are served via `//go:embed`.

Result rendering details, all client-side (the API still returns native JSON values):
- **Composite cells as colored YAML.** Object/array cell values are serialized to YAML by a tiny
  recursive emitter and wrapped in colorizing `<span>`s (keys vs. scalar types), matching the CLI
  table renderer's struct-cell presentation; scalars stay plain text. The `innerHTML` is built only
  from HTML-escaped content plus known span tags, so it is injection-safe.
- **URL + history.** Submitting pushes a `?sql=` history entry (only when the query changed);
  `popstate` restores the editor from the URL and re-runs; a `sql` param present on load pre-fills
  and runs. This makes queries bookmarkable and Back/Forward navigable.
- **Resizable columns.** Each `<th>` carries a drag handle; the table uses `table-layout: fixed` so
  dragging sets explicit per-column widths (with a minimum). Document-level mouse listeners track
  the drag and tear down on release.

### 5'. Browser launch & query pre-load

After the listener is bound, `UICommand.Run` opens the page in the default browser best-effort via
the platform launcher (`open` / `xdg-open` / `rundll32`); a launch failure is logged, not fatal.
An optional positional query is forwarded to the page through the URL's `sql` parameter (the
command does not execute it server-side). Unspecified bind hosts (`0.0.0.0` / `::`) are rewritten
to `127.0.0.1` for the opened URL.

### 4''. Asset minification (build-time only)

The editable front-end sources live under `internal/adapter/web/assets/`; `make web-assets`
minifies them into `internal/adapter/web/dist/`, which is what `//go:embed all:dist` ships in the
binary. **Generated minified output is never committed** — `dist/` is git-ignored; only a tracked
`dist/.gitkeep` keeps the directory present so the `all:dist` embed always compiles on a fresh
checkout. `make build`/`lint`/`test` depend on `web-assets`, and every CI job (lint, test, build,
release) regenerates assets before compiling, so the embedded files are always fresh from source.

The minifier (`github.com/tdewolff/minify`) is declared as a Go `tool` dependency and invoked via
`go tool`, so it is a **build-time-only** dependency — it is not imported by, or linked into, the
`kubectl-sql` binary. This keeps the embedded assets small (~50% off the JS) without growing the
runtime binary or adding a Node/bundler toolchain.

### 5. Mutation guardrail at the API boundary

Before executing, `POST /api/query` calls the existing delete-statement classifier; if it matches
(or any non-SELECT mutation is detected), it returns `403` without touching the cluster. This
enforces guardrail #6 (no browser-triggered writes) and is covered by a spec scenario.

### 6. Lifecycle & binding

Default bind `127.0.0.1:8080` (loopback only). `http.Server` started in a goroutine; main waits
on `signal.NotifyContext(SIGINT, SIGTERM)` then calls `Shutdown` with a short timeout. Startup
prints the URL to stderr (stdout stays clean). Bind failure returns a non-zero `api.ExitError`.

## Risks / Trade-offs

- **Exposing a query API on a port** → default bind is loopback (`127.0.0.1`); binding to a
  routable address is opt-in via `--ui-address`, and the API is read-only (DELETE rejected).
- **JSON renderer shape may not map cleanly to columns/rows for every query** (e.g. `SELECT *`
  with heterogeneous objects) → derive columns from the union/first-row keys and document that
  the table mirrors `--output json`; complex cells render as JSON text.
- **Reusing `Do`'s readline offset semantics over HTTP** → wrap it so the HTTP contract returns
  full candidate tokens, not readline-style suffixes, keeping the client simple.
- **No auth** → acceptable for a local debugging tool bound to loopback; documented as a
  non-goal, with a warning if the user binds to a non-loopback address.
- **Embedded assets increase binary size slightly** → assets are tiny (few KB of HTML/CSS/JS).

## Migration Plan

Purely additive. New flags default off (`--ui` absent → existing behavior unchanged). No data
migration. Rollback = revert the change; no persisted state. After archive, reconcile new specs
into `openspec/specs/` and run `/dev:update-readme-specs`.

## Open Questions

- Should `--ui-address` binding to a non-loopback host print a warning (or require an explicit
  `--ui-allow-remote` flag)? Leaning toward a printed warning for v1.
- Endpoint verb for completion: `GET /api/complete` (chosen) vs `POST` — revisit only if very
  long lines hit URL length limits.
