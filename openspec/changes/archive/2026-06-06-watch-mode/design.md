## Context

`kubectl-sql` executes queries through the octosql pipeline: `GetTable` → schema inference → typecheck → optimize → materialize → `Run` → `Render`. This pipeline is batch-oriented: it produces a full result table and exits.

Watch mode makes this a repeating loop. Every 5 seconds the exact same pipeline runs again and the output replaces the previous one on screen. No Kubernetes WATCH API, no event streaming, no custom row renderer — just polling.

## Goals / Non-Goals

**Goals:**
- `--watch` / `-w` flag activates a polling loop
- Each tick runs `runQuery` identically to a normal batch execution
- Output is cleared between ticks so the table appears to refresh in place
- All SQL clauses work normally: `WHERE`, `ORDER BY`, `LIMIT`, `GROUP BY`, aggregates
- All output formats work: `table`, `json`, `csv`
- Ctrl-C exits 0 cleanly
- `--timeout` caps the total watch duration

**Non-Goals:**
- Kubernetes WATCH API / event streaming (dropped — too complex, wrong UX)
- `RenderEvent`, `WatchHeader`, per-event output (dropped)
- Incompatibility detection for ORDER BY/LIMIT/aggregates (dropped — all clauses work)
- Screen diffing or highlighting changed rows

## Decisions

### Polling, not streaming

Watch mode re-runs `runQuery` on a 5-second ticker. This reuses 100% of the existing execution path. No new execution logic, no schema re-inference complexity, no partial row rendering. The output is always a complete, correct, fully-formatted result — identical to what the user gets without `--watch`.

### Screen clearing

Between ticks, the terminal is cleared using ANSI escape `\033[H\033[2J` (move cursor to top-left, clear screen). This works in any VT100-compatible terminal (all modern terminals). On non-TTY output (piped), clearing is skipped — each tick's output is simply appended.

### Implementation: wrap `runQuery` in a ticker loop

`runWatch` captures stdout into a buffer per tick, clears the terminal, then writes the buffer. This avoids interleaving partial table output with the clear sequence. The existing `runQuery` signature already writes to `os.Stdout` via `output.Render(opts{Writer: os.Stdout})` — we redirect the writer to a buffer for each tick.

### SIGINT handling

`signal.Notify` on `SIGINT`/`SIGTERM` cancels the context. The ticker loop exits on context cancellation. Exit code 0.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Flickering on slow clusters (query takes >1s) | Clear only after the new result is ready, not before the query starts |
| Non-TTY piped output gets cluttered with ANSI escapes | Detect `isatty` and skip clear sequence when not a TTY |
| 5-second interval is hardcoded | Acceptable for v1; `--interval` flag is future work |
