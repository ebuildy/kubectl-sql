## Context

`batch.OutputPrinter.Run()` calls `uilive.New()` unconditionally. `uilive.New()` calls `getTermSize()` which opens `/dev/tty` via `os.OpenFile`. On macOS, this syscall blocks when the process has no controlling terminal. The process prints results correctly but then hangs in `uilive.Flush()` or its cleanup path.

Secondary issue: octosql's `ristretto` caches spawn background goroutines that never stop, preventing `os.Exit` via normal return. Resolved by `os.Exit(0)` in `main.go`, which this change makes unnecessary by removing the octosql output path entirely.

## Goals / Non-Goals

**Goals:**
- Replace `batch.NewOutputPrinter` with `output.Render()` in `cmd/root.go`
- `output.Render()` drives the octosql execution node directly, with no TTY interaction
- Support `table`, `json`, `csv` output formats
- Remove `os.Exit(0)` workaround from `main.go`
- Keep ORDER BY and LIMIT working (collect all rows, sort, truncate)
- Keep aggregate functions (COUNT, SUM, etc.) working — watermark signal already sent

**Non-Goals:**
- Live/streaming output (not needed for k8s LIST queries)
- YAML output format (can be added later)
- Parallel record collection

## Design

### `internal/output/renderer.go`

```go
type Options struct {
    Format  string    // "table" | "json" | "csv"
    Limit   *int64
    OrderBy []execution.Expression
    OrderDirections []int
    Schema  physical.Schema
    Writer  io.Writer
}

func Render(ctx execution.ExecutionContext, node execution.Node, opts Options) error
```

`Render` calls `node.Run(ctx, produceFn, metaSendFn)` where:
- `produceFn` appends each record to a `[][]octosql.Value` slice
- `metaSendFn` is a no-op (watermark already handled by `kubernetesExecution`)

After `Run` returns:
1. Sort the slice if `OrderBy` is non-empty (same btree logic as OutputPrinter, but in a plain slice sort)
2. Apply `Limit` by truncating the slice
3. Render to the writer in the requested format

### Format implementations (in `internal/output/renderer.go`)

- **table**: `tablewriter` — same library octosql uses, no TTY interaction
- **json**: `encoding/json`, array of `map[string]interface{}`
- **csv**: `encoding/csv`

### `cmd/root.go` changes

Remove:
- `batch` and `formats` imports
- `batch.NewOutputPrinter` / `sink.Run()` block
- `execOrderBy` materialization (move sorting into `output.Render`)

Add:
- Call `output.Render(execCtx, execPlan, opts)` with format from `--output` flag

### `main.go` changes

Remove `os.Exit(0)` — process exits cleanly because no background goroutines from uilive remain, and ristretto goroutines are killed naturally when `os.Exit` isn't needed (the process simply returns).

Actually: ristretto goroutines are still spawned by `functions.FunctionMap()`. Keep `os.Exit(0)` in `main.go` as it correctly handles that unrelated leak.

## Tradeoffs

- **No live output**: OutputPrinter supports live-updating display as records stream in. We don't use `live=true` so this is already not active. No regression.
- **Memory**: We buffer all rows before rendering. For k8s LIST results this is fine — clusters rarely return millions of resources.
- **Sorting**: We use `sort.SliceStable` on the collected rows. Same O(n log n) as the btree approach, simpler code.
