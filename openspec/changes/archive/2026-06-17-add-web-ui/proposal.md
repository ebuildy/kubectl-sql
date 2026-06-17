## Why

`kubectl-sql` today is terminal-only (one-shot, REPL, batch). Some users — and especially
newcomers learning the SQL grammar — want a lightweight, no-install graphical way to type a
query, see syntax highlighting and completions, and read results as an HTML table without
parsing aligned terminal columns. A small embedded web UI gives this without adding a separate
deployment: the plugin already holds a live cluster connection, a SQL engine, and a completion
source, so it can serve a page locally on demand.

## What Changes

- Add a `--ui` flag that starts a local HTTP server instead of running a query, serving a
  single self-contained HTML page plus a small JSON API. The server uses the same cluster
  config (`--kubeconfig`, `--context`, `--namespace`), shuts down cleanly on Ctrl-C, and prints
  the listen URL on startup.
- Add `--ui-address` (default `127.0.0.1:8080`) to control the bind address.
- Serve a minimal, dependency-light single-page UI (vanilla JS, small embedded CSS — no
  React/Vue/build step) with two sections: a SQL editor textarea with syntax highlighting and
  autocomplete, and a results area that renders the JSON response as an HTML table. Object/array
  cells render as colored YAML (mirroring the CLI table renderer), and result columns are
  resizable by dragging the header edge.
- Reflect the submitted query in the page URL (`?sql=`) so it is bookmarkable and the browser
  Back/Forward buttons move between queries; a query present in the URL pre-fills the editor and
  runs on load.
- On startup, open the UI in the user's default browser (best-effort, non-fatal). When a positional
  query is passed alongside `--ui`, forward it to the page via `?sql=` so the editor opens
  pre-filled.
- Add `POST /api/query`: accepts `{ "sql": "..." }`, runs it through the existing SQL engine in
  JSON output mode, and returns `{ "columns": [...], "rows": [...] }` or a structured error
  (including the existing typo-correction suggestion when one is available).
- Add `GET /api/complete`: accepts the current editor line + cursor position and returns
  completion candidates, reusing the existing REPL completion source.
- Static assets (HTML/CSS/JS) are embedded in the binary via `go:embed` so `kubectl sql --ui`
  works with no external files.

This is additive and read-mostly: the UI exposes the same query path as the CLI. DELETE is **out
of scope** for v1 — the API rejects mutating statements so the browser cannot trigger
destructive operations without the CLI's confirmation flow.

## Capabilities

### New Capabilities
- `web-ui-command`: the `--ui` / `--ui-address` flags, HTTP server bootstrap, lifecycle
  (startup banner, graceful shutdown), and reuse of the existing cluster/SQL wiring.
- `web-ui-page`: the embedded single-page UI — SQL editor with syntax highlighting and
  autocomplete, results-as-HTML-table section, minimal CSS, no JS framework.
- `web-ui-api`: the JSON HTTP endpoints — `POST /api/query` and `GET /api/complete` — their
  request/response contracts, error shapes, and the DELETE/mutation rejection guardrail.

### Modified Capabilities
<!-- None. The query and completion ports are reused unchanged; no spec-level behavior of
     existing capabilities changes. -->

## Impact

- **New CLI flags**: `--ui`, `--ui-address` (registered in `cmd/root.go`).
- **New code** (hexagonal): a primary/driving adapter `internal/adapter/web` (HTTP server +
  embedded static assets + handlers) and a domain command `internal/domain/commands/ui` that
  wires the existing datasource, SQL engine, and completion adapters into the web adapter.
- **Reused unchanged**: `internal/port/sql` (Engine, JSON output), `internal/port/datasources/k8s`,
  `internal/port/autocomplete` + `internal/adapter/shell/completion`, `internal/port/api` Config,
  `octosqlAdapter.New` / `FunctionNames`, `k8sAdapter.New`.
- **Dependencies**: none new — uses Go stdlib `net/http` and `embed`.
- **Docs/specs**: README usage section; new long-lived specs reconciled on archive.
