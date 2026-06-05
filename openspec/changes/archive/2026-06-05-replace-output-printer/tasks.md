## 1. Renderer Implementation

- [x] 1.1 In `internal/output/renderer.go`, define `Options` struct with fields: `Format string`, `Limit *int64`, `OrderBy []execution.Expression`, `OrderDirections []int`, `Schema physical.Schema`, `Writer io.Writer`
- [x] 1.2 Implement `Render(execCtx execution.ExecutionContext, node execution.Node, opts Options) error` — calls `node.Run`, collects records into `[][]octosql.Value`, no-op metaSend
- [x] 1.3 After collection: apply ORDER BY via `sort.SliceStable` comparing octosql values using `.Compare()`
- [x] 1.4 After sort: apply Limit by truncating the slice
- [x] 1.5 Implement `renderTable(w io.Writer, schema physical.Schema, rows [][]octosql.Value)` using `tablewriter`
- [x] 1.6 Implement `renderJSON(w io.Writer, schema physical.Schema, rows [][]octosql.Value)` using `encoding/json`
- [x] 1.7 Implement `renderCSV(w io.Writer, schema physical.Schema, rows [][]octosql.Value)` using `encoding/csv`
- [x] 1.8 Dispatch to correct renderer based on `opts.Format`; return error for unknown format

## 2. Wire into cmd/root.go

- [x] 2.1 Remove imports: `batch`, `formats` from `cmd/root.go`
- [x] 2.2 Remove `sink := batch.NewOutputPrinter(...)` and `sink.Run(...)` block
- [x] 2.3 Remove `execOrderBy` materialization block (ORDER BY now handled in renderer)
- [x] 2.4 Read `--output` flag value in `runQuery`
- [x] 2.5 Build `output.Options` and call `output.Render(execution.ExecutionContext{Context: ctx}, execPlan, opts)`

## 3. Cleanup

- [x] 3.1 Verify `main.go` still has `os.Exit(0)` (needed for ristretto goroutine leak — keep it)

## 4. Tests

- [x] 4.1 Add unit tests in `internal/output/renderer_test.go` for `renderTable`, `renderJSON`, `renderCSV` with a mock node that produces 2 rows
- [x] 4.2 Add unit test verifying ORDER BY sorts correctly (2 rows, reversed order)
- [x] 4.3 Add unit test verifying LIMIT truncates correctly

## 5. Verification

- [x] 5.1 Run `go build ./...` — exits 0
- [x] 5.2 Run `make test` — exits 0
- [x] 5.3 Run `make lint` — exits 0
- [x] 5.4 Run `make e2e` — exits 0
- [x] 5.5 Run `make e2e-run-fake` — all 7 scenarios pass
- [x] 5.6 Run `./bin/kubectl-sql "SELECT count(*) FROM pods"` in a non-TTY environment — exits cleanly without hanging
