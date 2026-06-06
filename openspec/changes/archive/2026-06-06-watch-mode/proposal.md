## Why

`kubectl-sql` currently executes queries as a one-shot snapshot: it lists resources, streams results, and exits. Users who want to monitor changing cluster state (pods restarting, deployments rolling, events arriving) must re-run the query manually or wrap it in a shell loop. A `--watch` flag would make `kubectl-sql` a live monitoring tool, analogous to `kubectl get --watch` but with full SQL expressiveness.

## What Changes

- Add a `--watch` / `-w` flag to the root command
- When `--watch` is set, the query runs continuously using the Kubernetes WATCH API (not polling) — resource changes (ADDED, MODIFIED, DELETED) stream as events
- Each event prints a row with an extra `event` column (`ADDED` / `MODIFIED` / `DELETED`) prepended to the output
- WHERE filters apply to each incoming event, so `--watch "SELECT name, namespace FROM pods WHERE status->phase = 'Pending'"` only surfaces pending pods as they appear or change
- Output format: table (default, one row per event, no pagination), JSON (`--output json`, one JSON object per line — newline-delimited)
- SQL clauses incompatible with streaming are rejected with a clear error: `ORDER BY`, `LIMIT`, `GROUP BY`, `COUNT(*)` and other aggregates
- The watch stream runs until the user presses Ctrl-C (SIGINT) or the context times out
- `--timeout` is respected: if set, the watch runs for at most that duration

## Capabilities

### New Capabilities

- `watch-mode`: Live streaming of Kubernetes resource changes via the WATCH API, driven by a SQL WHERE filter, with per-event row output

### Modified Capabilities

- `sql-execution`: Accept `--watch` flag; validate that the query contains no streaming-incompatible clauses (ORDER BY, LIMIT, aggregates) when watch mode is active
- `output-renderer`: Support newline-delimited event output for watch mode (one row per event, no full-table rerender)

## Impact

- `cmd/root.go` — detect `--watch` flag, fork into watch execution path before octosql pipeline
- `internal/executor/executor.go` — new `Watch(ctx, gvr, namespace, produce)` method using `client.Resource(gvr).Watch()`
- `internal/output/renderer.go` — new `RenderEvent` function for single-row event output
- No new dependencies — `client-go` watch machinery is already available via `k8s.io/client-go/dynamic`
