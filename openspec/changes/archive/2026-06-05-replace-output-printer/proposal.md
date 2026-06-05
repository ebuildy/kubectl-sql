## Why

`batch.NewOutputPrinter` from octosql unconditionally calls `uilive.New()` which opens `/dev/tty` via a syscall. On macOS, this blocks indefinitely when the process has no controlling terminal (certain shell environments), causing `kubectl-sql` to hang after printing results and never exit. The `os.Exit(0)` workaround masks a secondary issue (ristretto goroutine leak) but does not fix the root cause.

## What Changes

- Remove `batch.NewOutputPrinter`, `uilive`, and `formats.NewTableFormatter` from `cmd/root.go`
- Implement a direct result renderer in `internal/output/renderer.go` that collects rows from the octosql execution plan and writes them to stdout without any TTY dependency
- Support `table` (default), `json`, `csv` output formats via the `--output` flag
- Remove the `os.Exit(0)` workaround — process exits cleanly via normal return

## Capabilities

### New Capabilities

- `output-renderer`: Direct result renderer in `internal/output/` — collects octosql records, applies ORDER BY and LIMIT, renders to table/json/csv without TTY interaction

### Modified Capabilities

- `sql-execution`: Output is now driven by our renderer, not octosql's `OutputPrinter`

## Impact

- `cmd/root.go` — remove `batch`/`formats`/`uilive` usage; call `output.Render()`
- `internal/output/renderer.go` — new implementation
- `go.mod` — `github.com/gosuri/uilive` becomes an indirect-only dep (octosql still pulls it but we don't use it)
- `main.go` — remove `os.Exit(0)` workaround
