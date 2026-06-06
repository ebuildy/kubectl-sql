## 1. CLI flag and query validation

- [x] 1.1 Add `--watch` / `-w` bool flag to `cmd/root.go` `init()`
- [x] 1.2 In `runQuery`, detect `--watch` flag and branch before the octosql pipeline
- [x] 1.3 Implement `validateWatchQuery(stmt sqlparser.Statement) error` — walks the AST and returns an error if `ORDER BY`, `LIMIT`, `GROUP BY`, or aggregate functions are present
- [x] 1.4 Call `validateWatchQuery` early in the watch path; print error to stderr and exit 1 on failure

## 2. Watch executor

- [x] 2.1 Add `Watch(ctx context.Context, gvr schema.GroupVersionResource, namespace string, produce func(eventType string, obj map[string]interface{}) error) error` to `internal/executor/executor.go` (or a new `watcher.go` file)
- [x] 2.2 Implement `Watch` using `dynClient.Resource(gvr).Watch(ctx, metav1.ListOptions{})` — loop over `resultChan`, call `produce` for each `watch.EventType` (ADDED / MODIFIED / DELETED), return on channel close or context cancellation
- [x] 2.3 Namespace-scope the watch call when `namespace != ""`

## 3. WHERE filter evaluation in watch mode

- [x] 3.1 In the watch path, infer schema via `CompositeInferrer` (same as batch path)
- [x] 3.2 Parse and typecheck the WHERE expression from the SQL AST against the inferred schema
- [x] 3.3 For each incoming event object, build an `octosql.Value` row using `resolveFieldValue` and evaluate the WHERE expression; skip the event if it does not match (or if there is no WHERE clause, pass all events)

## 4. Output rendering for watch events

- [x] 4.1 Add `RenderEvent(w io.Writer, format string, schema physical.Schema, eventType string, row []octosql.Value) error` to `internal/output/renderer.go`
- [x] 4.2 For `table` format: print the header once before the loop, then call `RenderEvent` per event as a plain row prefixed with `eventType`; flush after each row
- [x] 4.3 For `json` format: write each event as a single-line JSON object (NDJSON) with an `"event"` key prepended; flush after each line
- [x] 4.4 `csv` format in watch mode: return an unsupported error

## 5. Watch execution path in cmd/root.go

- [x] 5.1 Implement `runWatch(cmd *cobra.Command, query string) error` — extracts resource name from AST, resolves GVR, builds inferrer, parses/typechecks WHERE, calls `executor.Watch`, calls `RenderEvent` per event
- [x] 5.2 Print header row (column names) once before entering the watch loop for table format
- [x] 5.3 Handle SIGINT (Ctrl-C) gracefully: cancel the context, print a newline, exit 0
- [x] 5.4 Respect `--timeout`: derive context deadline from the flag value

## 6. Integration tests

- [x] 6.1 Add `createAndDeletePod` helper in `test/integration/fixture.go` for watch testing
- [x] 6.2 Add e2e scenario: `kubectl sql --watch "SELECT name FROM pods"` exits 0 after timeout
- [x] 6.3 Add e2e scenario: `ORDER BY` in watch mode exits 1 with error message
- [x] 6.4 Add e2e scenario: `LIMIT` in watch mode exits 1 with error message
- [x] 6.5 Add e2e scenario: `COUNT(*)` in watch mode exits 1 with error message

## 7. Verification

- [x] 7.1 `go build ./...` exits 0
- [x] 7.2 `make test` exits 0
- [x] 7.3 `make lint` exits 0
- [x] 7.4 `make e2e-run-fake` — all scenarios pass
