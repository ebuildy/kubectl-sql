## 1. SQL engine port

- [x] 1.1 Create `internal/port/sql/engine.go`: the `Query` struct (`SQL`, `Output`, `Namespace`, `PageSize`, `NoColor`) and the `Engine` interface (`Execute(ctx, Query, io.Writer) error`). MUST NOT import `github.com/cube2222/octosql`.
- [x] 1.2 Unit test the port is satisfiable library-free (`var _ Engine = (*fake)(nil)` with a fake in the test).

## 2. Move executor into the adapter

- [x] 2.1 Move `internal/executor/executor.go` into `internal/adapter/sql/octosql` (e.g. `database.go`): the `physical.Database` impl, `DatasourceImplementation`, execution `Node`, `toOctoFields`/`fieldToOctoType`, `resolveFieldValue`/`resolveStructValue`/`resolveMapAsStruct`/`anyToOctoValue`. Keep it consuming the k8s `DataSource` port.
- [x] 2.2 Move `internal/executor/resolver.go` (`ResolveField`) into the adapter (`resolver.go`).
- [x] 2.3 Move `internal/executor/*_test.go` into the adapter, adapting package name; keep the fake `DataSource` test.

## 3. Move output rendering into the adapter

- [x] 3.1 Move `internal/output/renderer.go` into the adapter (`render.go`): `Render`, table/json/csv writers, `valueToNativeTyped`/`valueToString`/`valueToNative`, `Options`. Unexport as needed (engine-internal).
- [x] 3.2 Move `internal/output/renderer_test.go` into the adapter, adapting package + imports.

## 4. Move the pipeline + rewriter into the engine

- [x] 4.1 Create `internal/adapter/sql/octosql/engine.go`: `New(ds k8sport.DataSource) sql.Engine` and `Execute(ctx, q sql.Query, w)`. Move the pipeline from `cmd.runQueryWithWriter` (rewrite → parse → ParseNode → typecheck → optimize → ORDER BY/LIMIT → materialize → render) here, plus `typecheckNode`/`typecheckExpr`.
- [x] 4.2 Move `rewriteQuery`/`rewriteDottedFields` and the regexes (`dottedWildcardRe`, `dottedFieldRe`, `arrayIndexPathRe`) into the adapter (`rewrite.go`); move their unit tests (`cmd/rewrite_test.go`) too.
- [x] 4.3 Merge the existing `functions.go` custom `FunctionMap` with octosql's base map inside the engine (as `cmd` does today).
- [x] 4.4 Keep the per-step debug logs (rewritten/parsed/typechecked/optimized/completed) in the engine via `logger.FromContext(ctx)`.

## 5. Rewire cmd

- [x] 5.1 `cmd/root.go` `runQueryWithWriter`: after `SHOW TABLES`/`DESCRIBE TABLE` routing (unchanged, via k8s port), build `ds := k8sadapter.New(...)`, `eng := octosqladapter.New(ds)`, and call `eng.Execute(ctx, sql.Query{SQL: query, Output: ..., Namespace: ..., PageSize: ..., NoColor: ...}, w)`.
- [x] 5.2 Remove all octosql imports and the moved helpers (`rewriteQuery`, `typecheckNode`, `typecheckExpr`, pipeline) from `cmd`. Keep the `os.Exit` ristretto comment in `cmd.Execute`.
- [x] 5.3 Confirm REPL (`RunQuery` closure) and `runWatch` route through `Execute` (they call `runQueryWithWriter`, so they inherit it).
- [x] 5.4 Move `cmd/show_tables_test.go` expectations if any reference removed helpers; keep SHOW TABLES/DESCRIBE behavior.

## 6. Remove old packages

- [x] 6.1 Delete `internal/executor/` once nothing imports it.
- [x] 6.2 Delete `internal/output/` once nothing imports it. Update all imports.

## 7. Boundary + regression

- [x] 7.1 Add `internal/adapter/sql/octosql/boundary_test.go`: scan the tree; assert `github.com/cube2222/octosql` is imported only under `internal/adapter/sql/octosql` (exclude `test/`).
- [x] 7.2 Run `make lint e2e`; fix issues.
- [x] 7.3 Run unit tests and the envtest integration suite — all existing scenarios MUST pass unchanged (no behavior change).
