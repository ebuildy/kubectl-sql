## Why

Today `kubectl-sql` is strictly read-only — users can find broken resources with a query
(e.g. pods stuck in `Pending`) but then have to switch to `kubectl delete` and retype the
selection by hand. Allowing the same SQL `WHERE` filter to drive a deletion closes that loop
and makes cleanup a single command.

> ⚠️ **This intentionally breaches the read-only guardrail.** AGENTS.md guardrail #6 states the
> plugin must never write, patch, delete, or exec. This proposal deliberately introduces the
> first write path (DELETE only). It is flagged explicitly here, gated behind an interactive
> confirmation, and scoped to deletion alone — no create/update/patch/exec.

## What Changes

- Add a new `DELETE` statement to the query grammar:
  `DELETE [FROM] <resource> [WHERE <expr>]` (e.g. `DELETE pod WHERE status->phase = 'Pending'`).
- Introduce a new **`mutator` SQL adapter** (`internal/adapter/sql/mutator`), a sibling to the
  existing `octosql` adapter, that owns mutating statements (DELETE now; UPDATE later). octosql
  stays SELECT-only and is never asked to parse DELETE.
- The query is routed by statement kind: SELECT-family goes to the octosql adapter, DELETE goes
  to the mutator adapter. The mutator adapter resolves which objects to act on by delegating to the
  octosql adapter — it runs `SELECT namespace, name FROM <resource> [WHERE <expr>]` — so the
  `WHERE` semantics are identical to a SELECT, with no duplicated filter logic.
- **Display the namespace + name of every resource that will be deleted** and **ask for
  interactive confirmation** before any object is removed. Default answer is "no".
- Add a `--yes` / `-y` flag to skip the prompt for scripted use. In a non-interactive session
  (piped stdin / no TTY) deletion is refused unless `--yes` is given.
- Support `kubectl delete`-style options via a **MySQL-style hint comment** placed right after
  `DELETE`, e.g. `DELETE /* force, grace-period=0 */ FROM pod WHERE ...`. Recognised hints:
  `force`, `grace-period=<n>` (seconds), and `cascade=background|foreground|orphan`. These map
  to Kubernetes `DeleteOptions` (grace period, propagation policy). Unknown hints are a parse
  error (exit 1).
- Add a `Delete` method (taking domain `DeleteOptions`) to the Kubernetes `DataSource` port and
  its client-go adapter (using the dynamic client's `Delete`), confined to the adapter as all
  k8s.io code is.
- On confirmation, delete the matched objects **concurrently, capped at 10 in flight** (a single
  k8s delete can take seconds), wait for all to finish, then print a per-object status summary
  (deleted / failed) once at the end; exit 0 if all succeeded, 2 on any API error.
- Show a live **progress bar** (`github.com/schollz/progressbar/v3`) during deletion for one-shot
  CLI runs on a TTY; suppressed in the REPL and for non-interactive output.
- Define DELETE's interaction with existing run modes: reject `DELETE` with `--watch` (exit 1),
  treat `--dry-run` as preview-only (print the plan, delete nothing, exit 0), and support DELETE
  inside the REPL (prompt read from the REPL input).
- **BREAKING (policy):** the plugin is no longer exclusively read-only.

## Capabilities

### New Capabilities
- `delete-statement`: the `DELETE` grammar (including the hint-comment delete options),
  dry-run preview of the deletion set, interactive confirmation flow, `--yes` flag, and
  exit-code contract.
- `sql-mutator-adapter`: the structural contract for the new `mutator` SQL adapter — it handles
  mutating statements, delegates row resolution to the octosql SELECT engine, performs mutations
  through the `DataSource` port, and is the only place DELETE/UPDATE logic lives.

### Modified Capabilities
- `k8s-datasource-port`: the `DataSource` port gains a `Delete` operation taking a domain
  `DeleteOptions` (grace period, force, propagation policy) and the adapter an implementation,
  making the port no longer list/read-only.
- `sql-execution`: `DELETE` is intercepted ahead of the octosql pipeline (like `SHOW TABLES` /
  `DESCRIBE TABLE`) and routed to the new `mutator` SQL adapter.

## Impact

- Code: `cmd/root.go` (new `--yes` flag), `internal/domain/commands/query/*` (DELETE routing,
  preview, confirmation prompt, progress bar), `internal/adapter/sql/mutator` (new mutator
  adapter), a mutator port in `internal/port/sql`, `internal/port/datasources/k8s` (port method),
  and `internal/adapter/datasources/k8s` (dynamic-client delete).
- Dependency: adds `github.com/schollz/progressbar/v3` (MIT), used only on the `runDelete`
  rendering path — see design.md guardrail-#3 justification.
- Security posture: introduces the first mutating cluster call. RBAC `delete` verb is now
  required for the targeted resource; without it the call fails with a k8s API error (exit 2).
- Docs: README debug recipes and grammar reference gain a DELETE example; `AGENTS.md`
  guardrail #6 must be reconciled to note the sanctioned DELETE exception during archive.
- No change to existing SELECT behavior, output formats, or flags.
