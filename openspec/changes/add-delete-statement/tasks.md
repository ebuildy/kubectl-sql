## 1. DataSource port: delete operation

- [ ] 1.1 Add a domain `DeleteOptions` struct (`GracePeriodSeconds *int64`, `PropagationPolicy string`) and `Delete(ctx context.Context, r Resource, namespace, name string, opts DeleteOptions) error` to the `DataSource` interface in `internal/port/datasources/k8s`
- [ ] 1.2 Add a no-op `Delete` to the `fakeDataSource` in the port's `_test.go` so `TestPortIsSatisfiable` still compiles
- [ ] 1.3 Implement `Delete` in `internal/adapter/datasources/k8s/datasource.go` using the dynamic client (`d.dyn.Resource(gvrFor(r))`), choosing namespaced vs cluster-scoped via `r.Namespaced`, translating `DeleteOptions` into `metav1.DeleteOptions` (`GracePeriodSeconds`, `PropagationPolicy`), wrapping errors with `fmt.Errorf("k8s: delete %s/%s: %w", ...)`
- [ ] 1.4 Confirm the import-boundary test (`boundary_test.go`) still passes — no k8s.io types in the port signature

## 2. Mutator SQL port

- [ ] 2.1 Add a `Mutator` interface to `internal/port/sql` with `Plan(ctx, sql string) (DeletePlan, error)` and `Apply(ctx, DeletePlan, onProgress func()) (DeleteResult, error)`, plus the domain types `DeletePlan` (targets `[]ObjectRef{Namespace, Name}`, `Resource`, `DeleteOptions`, `KubectlCommands []string`) and `DeleteResult` (per-object outcomes). Keep the port library-free (no octosql, no k8s.io, no progressbar types)

## 3. Mutator SQL adapter

- [ ] 3.1 Create `internal/adapter/sql/mutator` with a constructor injecting the octosql `Engine` port and the k8s `DataSource` port; the package must import neither octosql internals nor client-go
- [ ] 3.2 Add `parseDelete(query string)` that detects a leading `delete` token (case-insensitive), extracts and strips an optional `/* ... */` hint comment, strips an optional `FROM`, and returns the resource name + optional WHERE tail + parsed `DeleteOptions` (or a parse error)
- [ ] 3.3 Add `parseDeleteHints(body string)` that splits the comment body on commas and maps `force`, `grace-period=<n>`, `cascade=background|foreground|orphan` (case-insensitive) into `DeleteOptions`, returning a parse error for unknown tokens or malformed values
- [ ] 3.4 Add `deleteOptionsToFlags(DeleteOptions) []string` rendering `--force`, `--grace-period=<n>`, `--cascade=<policy>` — the single source of truth for both the preview command lines and the applied options
- [ ] 3.5 Implement `Plan`: parse the DELETE, resolve the `Resource` (via the injected DataSource), delegate `SELECT namespace, name FROM <resource> [WHERE <tail>]` to the octosql `Engine` into a buffer using a fixed structured format, parse back the rows, and build the `DeletePlan` (targets + `kubectl delete <kind> <name> [-n <ns>] [flags]` lines via `deleteOptionsToFlags`)
- [ ] 3.6 Implement `Apply`: delete targets concurrently bounded to 10 in flight (buffered-channel semaphore or `errgroup.SetLimit(10)`), calling `DataSource.Delete` per target with the plan's `DeleteOptions`, recording each outcome race-free into a position-indexed `DeleteResult`, invoking `onProgress` (if non-nil) once per completed delete, waiting for all before returning; no user printing or progressbar import in `Apply`
- [ ] 3.7 Unit-test `parseDelete`/`parseDeleteHints`/`deleteOptionsToFlags` for: `DELETE pod WHERE ...`, `DELETE FROM pods WHERE ...`, `DELETE pods` (no WHERE), `DELETE /* force, grace-period=0 */ FROM pod ...`, `DELETE /* cascade=orphan */ ...`, `DELETE WHERE ...` (error), `DELETE /* bogus */ pods` (error), and each flag-rendering case

## 4. Deletion flow in QueryCommand

- [ ] 4.1 In `RunWithWriter`, detect a leading `delete` token (alongside the `show tables` / `describe table` checks) and route to a new `runDelete(ctx, query)` method that uses the mutator adapter
- [ ] 4.2 Reject DELETE combined with `--watch` before resolving anything: print a conflict error and return an exit-1 error (guard added in `runWatch`/routing)
- [ ] 4.3 In `runDelete`, call `mutator.Plan`; print the preview — namespace + name + total count and the exact `kubectl delete ...` line per object; if the set is empty, print "nothing matched" and return nil (exit 0)
- [ ] 4.4 When `--dry-run` is set, stop after printing the preview — no prompt, no `Apply` — and return nil (exit 0)
- [ ] 4.5 Implement the confirmation gate: on TTY prompt yes/no (default no), reading the answer through the REPL input reader when invoked from the REPL (not a second `os.Stdin` scanner); honour the new `--yes` flag; refuse with exit-1 error when non-interactive and `--yes` is unset
- [ ] 4.6 On confirmation, construct a `progressbar/v3` bar (total = matched count) only for one-shot CLI runs on a TTY — suppressed in the REPL and for non-interactive output — and call `mutator.Apply` with `onProgress` wired to `bar.Add(1)` (or a no-op when suppressed)
- [ ] 4.7 Once `Apply` returns, finish/clear the bar, print the per-object status table (preview order) and a deleted/failed count summary, and return an error that maps to exit 2 if any delete failed
- [ ] 4.8 Add an `inREPL bool` field to `QueryCommand` (set true on the `NewQueryCommandWithDataSource` REPL path, false for the one-shot root command) so `runDelete` can suppress the progress bar and read confirmation from the REPL reader

## 5. CLI flag and dependency

- [ ] 5.1 Add the persistent `--yes` / `-y` bool flag in `cmd/root.go` and thread it through `api.Config` into `QueryCommand`
- [ ] 5.2 Add `github.com/schollz/progressbar/v3` to `go.mod` (run `go get`); ensure it is imported only on the `runDelete` rendering path, not by the mutator adapter or any port

## 6. Tests

- [ ] 6.1 Add a mutator-adapter test using a fake `Engine` (returns canned namespace/name rows) and a fake `DataSource` (recording `Delete` calls): verify `Plan` delegates the SELECT and builds correct kubectl command lines, and `Apply` deletes every target with the parsed options
- [ ] 6.2 Add a query-command test: preview is printed, decline deletes nothing, `--yes` deletes all matched, empty match is a no-op
- [ ] 6.3 Add a test that non-interactive (non-TTY) DELETE without `--yes` exits 1 and deletes nothing
- [ ] 6.4 Add a test that a failing `Delete` produces the failed summary and the exit-2 error mapping
- [ ] 6.5 Add tests that DELETE + `--watch` exits 1 and deletes nothing, and that DELETE + `--dry-run` prints the preview, prompts nothing, and deletes nothing
- [ ] 6.6 Add an `Apply` concurrency test: with a fake `DataSource.Delete` that blocks on a counter, assert no more than 10 calls run simultaneously over a >10 target set, all targets are attempted, outcomes are returned in preview order, and `onProgress` fires exactly once per target (run under `-race`)

## 7. Docs & verification

- [ ] 7.1 Add `DELETE` examples (including the hint-comment options form) to the README debug recipes and to `docs/grammar.ebnf`
- [ ] 7.2 Run `make lint build` and `make test` — all green
