# ADR-005: Watch Mode — Polling vs. Kubernetes WATCH Events

**Date:** 2026-06-06  
**Status:** Accepted  
**Deciders:** Thomas Decaux

---

## Context

`--watch` mode needs to keep the terminal display up to date as cluster state changes. Two approaches exist:

1. **Polling** — re-run the full SQL query every N seconds, clear the screen, reprint the table
2. **WATCH events** — open a long-lived Kubernetes WATCH stream, receive `ADDED / MODIFIED / DELETED` events, and maintain a live view incrementally

A first implementation of watch mode attempted the event-streaming approach. It was abandoned mid-implementation because of correctness and complexity problems (see Consequences). Polling was chosen as the replacement.

---

## Decision

Use **polling** (5-second interval, re-run full query) for watch mode v1.

---

## Rationale

### Polling is correct by construction

Each tick runs the exact same octosql pipeline as a normal batch query: schema inference → typecheck → paginated LIST → SQL filter/sort/aggregate → render. The output is provably identical to running the query manually. There is no separate code path to test, no partial state to maintain, no divergence risk.

### Event streaming is fundamentally more complex

Implementing watch mode via Kubernetes WATCH events requires:

- **A live state map**: events are deltas — to answer `SELECT name, status FROM pods` you must maintain a local copy of the full resource set and update it on each event
- **Re-evaluation of the full SQL query against in-memory state** after each event, OR per-event WHERE evaluation that bypasses the octosql pipeline
- **Initial burst handling**: the WATCH API sends an `ADDED` event for every existing object before streaming changes — this must be distinguished from genuine additions
- **Reconnect logic**: Kubernetes watches expire after ~5 minutes; the client must detect channel closure and reopen the watch, re-seeding the local state map
- **Schema consistency**: if the schema changes between reconnects (e.g. a CRD is updated), the state map may contain stale typed values

Each of these is a source of bugs. The first implementation demonstrated this: `ADDED <null> <null>` output caused by `octosql`'s `table.column` naming convention not being stripped correctly when resolving fields outside the normal pipeline.

### The user experience is the same

For the typical `kubectl-sql` use case — watching a handful of pods, deployments, or events — a 5-second poll is indistinguishable from event streaming. The cluster state does not change faster than the poll interval in most debugging scenarios.

### Polling respects all SQL clauses for free

`ORDER BY`, `LIMIT`, `GROUP BY`, aggregates — all work in watch mode with no extra handling because the full octosql pipeline runs on every tick. With event streaming, these clauses are fundamentally incompatible with incremental updates and would need to be rejected or reimplemented.

---

## Consequences

- **Watch interval is fixed at 5 seconds** — no `--interval` flag in v1. Acceptable for interactive debugging.
- **Each tick issues a full paginated LIST** — on large clusters (10k+ pods) this may be slow or produce significant API server load. This is the primary reason to consider event streaming in the future.
- **No diff highlighting** — the table is replaced whole on each tick. Changed rows are not highlighted. This is a UX limitation of the polling approach.
- **TTY detection required** — the ANSI clear-screen sequence (`\033[H\033[2J`) is only emitted when stdout is a TTY, detected via `golang.org/x/term`. Piped output appends ticks without clearing.

---

## Future: event streaming for large clusters

Kubernetes WATCH events are the right solution when:

- The cluster is too large for per-tick LIST calls to be practical (thousands of resources, paginated across multiple pages)
- Sub-second latency on changes is required
- Diff highlighting (highlight changed rows) is desired

A future `--watch-events` flag (or automatic threshold-based switching) could implement event streaming as a separate execution path alongside polling. The `SchemaInferrer` and field resolver infrastructure are already in place; the main work would be the state map and reconnect logic.

The polling implementation does not block this — the two modes can coexist.
