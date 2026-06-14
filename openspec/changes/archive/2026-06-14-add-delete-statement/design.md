## Context

`kubectl-sql` is read-only by design (AGENTS.md guardrail #6). SQL execution flows through the
octosql engine (`internal/adapter/sql/octosql`, behind the `internal/port/sql` `Engine` port),
which only understands `SELECT`. Non-SELECT statements are handled by string interception in
`QueryCommand.RunWithWriter` *before* octosql is invoked — this is already how `SHOW TABLES` and
`DESCRIBE TABLE` work. The Kubernetes integration is a hexagonal `DataSource` port
(`internal/port/datasources/k8s`) whose only library binding lives in the adapter
(`internal/adapter/datasources/k8s`), which already holds a `dynamic.Interface` client and a
REST mapper used for `List`.

This change adds the first mutating path: `DELETE`. Since octosql cannot parse DELETE/UPDATE, the
mutating path gets its own SQL adapter (`mutator`) rather than ad-hoc string handling in the domain
command. It must reuse the existing WHERE semantics (via octosql), must show the user exactly what
will be removed, and must not delete without explicit consent.

## Goals / Non-Goals

**Goals:**
- Support `DELETE [FROM] <resource> [WHERE <expr>]` with WHERE semantics identical to SELECT.
- House mutating statements in a new `mutator` SQL adapter that reuses octosql for row resolution
  and the `DataSource` port for the mutation, keeping octosql SELECT-only.
- Support `kubectl delete` options via a MySQL-style hint comment after `DELETE`
  (`force`, `grace-period=<n>`, `cascade=...`), mapped to k8s `DeleteOptions`.
- Always preview namespace + name of every target before any deletion, and print the exact
  equivalent `kubectl delete <kind> <name> -n <ns> [--force --grace-period=<n> --cascade=<p>]`
  command that will run on confirmation.
- Require interactive confirmation (default "no"); offer `--yes` for scripted use.
- Add a single `Delete` method to the `DataSource` port + adapter, keeping client-go confined.
- Honour `--namespace`, exit 0 on success/decline, 1 on parse/usage error, 2 on API failure.

**Non-Goals:**
- No CREATE / UPDATE / PATCH / APPLY / EXEC — DELETE only.
- No field/label-selector-only forms beyond what WHERE already expresses (can be future changes).
- No per-object option overrides — hint options apply uniformly to the whole deletion set.
- No configurable concurrency limit in v1 — the parallelism cap is fixed at 10.
- No new octosql grammar — DELETE never reaches octosql.

## Decisions

### New `mutator` SQL adapter owns mutating statements
octosql is SELECT-only, so rather than scatter DELETE handling through `QueryCommand`, mutating
statements get their own SQL adapter: `internal/adapter/sql/mutator`, a sibling of
`internal/adapter/sql/octosql`. The mutator is the single home for DELETE (and future UPDATE)
logic. `QueryCommand.RunWithWriter` only *routes*: it detects a leading `delete` token
(case-insensitive), alongside the existing `show tables` / `describe table` checks, and dispatches
to the mutator adapter; everything else still goes to octosql.

The mutator adapter is wired from two dependencies via its constructor: the octosql `Engine` port
(to run SELECT) and the k8s `DataSource` port (to delete). It therefore imports neither octosql
internals nor client-go — both stay behind their ports. A small `Mutator` interface in
`internal/port/sql` exposes the flow as two steps so the interactive preview/confirmation stays in
the domain layer:

- `Plan(ctx, sql) (DeletePlan, error)` — parse the DELETE (resource, WHERE tail, hint options),
  then **delegate to octosql**: run `SELECT namespace, name FROM <resource> [WHERE <tail>]` into an
  in-memory buffer and read back the rows. Returns the resolved targets, the parsed `DeleteOptions`,
  and the per-object equivalent `kubectl delete` command lines. No mutation happens here.
- `Apply(ctx, DeletePlan) (Result, error)` — call `DataSource.Delete` per target with the options
  and collect per-object results.

`QueryCommand.runDelete` calls `Plan`, prints the preview, runs the confirmation gate, then calls
`Apply`. This keeps TTY interaction out of the adapter while the adapter owns all SQL/mutation
logic.

*Why reuse octosql for resolution:* the deletion set is exactly what `SELECT namespace, name
FROM ...` returns — identical operators, arrow paths, and helper functions — so there is zero
duplicated filter logic.

*Alternative considered:* add a filter/predicate method to the DataSource port and evaluate WHERE
ourselves. Rejected — it would duplicate octosql's expression engine and drift from SELECT
semantics.

*Alternative considered:* keep DELETE inline in `QueryCommand` (no new adapter). Rejected — it
mixes mutating-statement parsing/execution into the domain command and leaves no clean seam for
UPDATE; a dedicated adapter mirrors the existing octosql adapter and keeps the hexagon clean.

*Implementation note:* the delegated SELECT must produce machine-readable rows (namespace + name)
regardless of `--output`. The mutator runs it with a fixed structured format (e.g. internal
`csv`/`json`) into a buffer rather than the user's chosen table format, so parsing is robust.

### Delete options via MySQL-style hint comment
kubectl-delete options ride in a `/* ... */` comment immediately after `DELETE`. The hint parser
extracts the comment body, splits on commas, and reads each token as a bare flag or `key=value`
(names case-insensitive, whitespace-trimmed). Recognised tokens populate a domain `DeleteOptions`:

- `force` → grace period 0 (immediate deletion).
- `grace-period=<n>` → `GracePeriodSeconds = n`.
- `cascade=background|foreground|orphan` → propagation policy.

Unknown tokens or malformed values are a parse error (exit 1) — fail loud rather than silently
ignore a destructive-intent option. The comment is stripped before the remaining
`DELETE [FROM] <resource> [WHERE ...]` is parsed, and is never forwarded to the synthesised
SELECT.

*Why a hint comment (not flags):* the option travels with the statement, so it survives REPL
history and piped batch input where there is no place to attach a CLI flag, and it mirrors the
familiar MySQL optimizer-hint syntax. CLI flags would also be ambiguous when multiple statements
are piped. The domain `DeleteOptions` keeps k8s `metav1.DeleteOptions` out of the port.

### Add `Delete(ctx, Resource, namespace, name, DeleteOptions)` to the DataSource port
The adapter already has the `dynamic.Interface` and `gvrFor(r)` used by `List`. `Delete` mirrors
`List`: build the resource interface, then call `ri.Namespace(ns).Delete(...)` (or cluster-scoped
`ri.Delete(...)` when `Resource.Namespaced` is false), translating the domain `DeleteOptions`
into `metav1.DeleteOptions` (`GracePeriodSeconds`, `PropagationPolicy`) and wrapping errors with
context. The port method takes only domain types (`Resource`, two strings, `DeleteOptions`), so
no k8s.io type crosses the boundary — preserving the `k8s-datasource-port` boundary test. The
fake in the port test gains a no-op `Delete` to keep satisfying the interface.

### Preview renders the exact kubectl delete command per object
The mutator's `DeletePlan` carries, for each matched object, the equivalent `kubectl delete <kind>
<name> [-n <ns>] [flags]` line, which `runDelete` prints so the user sees precisely what will run
before confirming. Because the same `DeletePlan` (same targets, same `DeleteOptions`) is what
`Apply` feeds to `DataSource.Delete`, the printed command cannot drift from what executes — a
single `deleteOptionsToFlags(DeleteOptions) []string` helper in the mutator renders the option
flags (`--force`, `--grace-period=<n>`, `--cascade=<policy>`) for both. The kind comes from the
resolved `Resource`; the namespace flag is omitted for cluster-scoped resources. This is
presentational (kubectl is not shelled out); the actual deletion goes through the `DataSource.Delete`
port.

### Confirmation flow and `--yes` flag
After printing the preview table, on a TTY the command reads a yes/no answer from stdin
(default no). `--yes`/`-y` (new persistent flag in `cmd/root.go`) skips the prompt. TTY
detection reuses the existing `utils.StdinIsTTY()` helper. Non-interactive + no `--yes` → refuse
and exit 1, so piped/batch usage can never silently delete. Empty match set → no prompt, exit 0.

### Interaction with execution modes (watch, dry-run, REPL)
DELETE has to behave sanely against the existing run modes, so routing checks them up front:

- **`--watch`**: rejected. `runWatch` re-executes the query every 5s; a confirmed one-shot
  mutation in a poll loop is nonsensical (and after the first run nothing would match). The router
  returns an exit-1 error before resolving anything when both `--watch` and a DELETE are present.
- **`--dry-run`**: preview-only. The flag already means "validate without hitting the API". For
  DELETE we resolve and print the plan (including the `kubectl delete` lines) but skip both the
  confirmation prompt and the `Apply` call, exiting 0. This makes `--dry-run` the
  non-interactive way to *see* what a DELETE would do without `--yes` risk.
- **REPL**: DELETE already flows through `QueryCommand.RunWithWriter`, which the REPL calls per
  line, so routing is automatic. The one subtlety is the confirmation prompt sharing stdin with
  the REPL's line reader; `runDelete` must read the answer through the REPL's input reader (passed
  via config/context) rather than opening a second `bufio.Scanner` on `os.Stdin`, to avoid
  swallowing the next REPL line. In non-interactive batch REPL (piped stdin), the same
  `--yes`-required rule applies.

*Alternative considered:* treat `--dry-run` as a no-op for DELETE (since the preview is already an
always-on dry-run before confirm). Rejected — wiring `--dry-run` to "preview and stop" gives
scripts a safe, promptless way to inspect the deletion set, which the confirmation gate alone does
not.

### Apply deletes in parallel, bounded to 10, results shown at the end
A single `DataSource.Delete` can take seconds (finalizers, graceful termination), so `Apply`
deletes targets concurrently rather than one at a time. Concurrency is bounded to **10 in flight**
via a buffered-channel semaphore (or `errgroup` with `SetLimit(10)`); every target is dispatched,
each goroutine records its own `(namespace, name, err)` outcome into a slice indexed by position
(no shared mutation, so no lock contention), and `Apply` waits for all to finish. The context
timeout still applies to each delete.

The per-object result lines are **not** streamed as deletes complete (which would interleave
unpredictably under concurrency); instead `Apply` returns the full `DeleteResult` and `runDelete`
prints the status table once, in the original preview order, after everything has settled. This
keeps output deterministic and readable.

*Why cap at 10:* enough to hide per-call latency on a large match set without hammering the API
server or tripping client-side rate limits; a fixed constant avoids a new flag in v1.

### Progress bar during deletion (non-REPL only)
Because a large deletion set can take a while, `runDelete` shows a live progress bar that advances
as each delete completes, using `github.com/schollz/progressbar/v3`. To keep `Apply` UI-free, the
adapter takes an `onProgress func()` callback and invokes it once per finished delete (from each
goroutine — `progressbar/v3` is safe for concurrent `Add`); `runDelete` wires that callback to
`bar.Add(1)`. The bar's total is the matched-object count from the plan.

The bar is shown **only for one-shot CLI runs on a TTY**: it is suppressed in the REPL (its
redraws would fight the line editor) and whenever the deletion output target is not an interactive
terminal (piped output, watch buffer — though DELETE+watch is already rejected). `QueryCommand`
learns it is in the REPL via an `inREPL` field set by the REPL wiring (`NewQueryCommandWithDataSource`
path); the one-shot root command leaves it false. When suppressed, deletion still runs identically —
only the bar is omitted — and the end-of-run status table is always printed.

*Dependency:* `github.com/schollz/progressbar/v3` (MIT) — a small, widely-used, dependency-light
terminal progress bar. Justified here per guardrail #3; it is confined to the `runDelete` rendering
path and the mutator adapter never imports it (it only calls the `onProgress` callback).

### Exit codes
Parse/usage errors (no resource, non-interactive without `--yes`) → exit 1, consistent with the
existing query/parse error code. Any delete API failure → exit 2, matching the documented "k8s
API error" code. All deletes are attempted (concurrently) even if some fail, so the user sees the
full result set.

## Risks / Trade-offs

- **Destructive operation in a tool that was read-only** → Mitigated by mandatory preview,
  default-no confirmation, required `--yes` for non-interactive, DELETE-only scope, and an
  explicit flag in the proposal. AGENTS.md guardrail #6 is reconciled at archive time to record
  the sanctioned exception.
- **TOCTOU: an object may change/vanish between preview and delete** → Acceptable; a delete of an
  already-gone object surfaces as a per-object error in the summary and is not fatal to the rest.
- **Concurrent deletes race on shared result state** → Each goroutine writes only its own slot in
  a position-indexed result slice (no shared mutable map); verified with a `-race` test (task 6.6).
- **10-way parallelism could pressure the API server / hit client rate limits** → Cap is a fixed
  conservative constant; deletes are independent single-object calls, not a thundering herd.
- **RBAC lacks the `delete` verb** → Surfaces as a k8s API error per object, exit 2; message is
  enriched to suggest the missing permission.
- **WHERE that matches more than intended** → The preview is the safeguard; the user sees every
  namespace+name and count before confirming.
- **Synthesised SELECT output parsing fragility** → Mitigated by using a fixed structured format
  (not the user's `--output`) and selecting only `name, namespace`.

## Migration Plan

Additive change. No existing behavior changes; SELECT, output formats, and flags are untouched.
The new `--yes` flag defaults to false. Rollback is removal of the interception branch, the port
method, and the flag. README and grammar docs gain a DELETE example during implementation.

## Open Questions

None. `--namespace` is **not** required for DELETE: it follows SELECT semantics (all namespaces
unless `-n` is passed). The mandatory preview + confirmation prompt is a sufficient guard, so no
extra namespace gate is added.
