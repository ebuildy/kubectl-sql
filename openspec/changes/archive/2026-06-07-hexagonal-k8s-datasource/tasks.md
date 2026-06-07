## 1. Port-side domain types

- [x] 1.1 Create `internal/port/schema/field.go`: move the library-free `Field`, `FieldType` (+ constants) and the ignored-field set/helper from `internal/schema/port.go`. No `k8s.io/*` imports.
- [x] 1.2 Move `internal/schema/walk.go` (pure, no apimachinery) and `internal/schema/inferrer.go` helpers that don't import k8s into `internal/port/schema` (or an adapter-internal helper if they're only used by inferrers). Keep them library-free where placed in the port package.

## 2. k8s data-source port

- [x] 2.1 Create `internal/port/datasources/k8s/datasource.go`: `Resource` struct, `ListOptions`, and the `DataSource` interface (`Resolve`, `Resources`, `InferSchema`, `List(...pageFn)`). MUST NOT import any `k8s.io/*` package; uses `internal/port/schema` for fields.
- [x] 2.2 Unit test the port compiles library-free (a trivial `var _ DataSource = (*fake)(nil)` with a fake implementation in the test).

## 3. k8s adapter

- [x] 3.1 Create package `internal/adapter/datasources/k8s`. Move `internal/k8s/client.go` (dynamic client, REST mapper, discovery) here as unexported helpers.
- [x] 3.2 Move the inferrers (`openapi_inferrer.go`, `sample_inferrer.go`, `composite_inferrer.go`) into the adapter; adapt them to produce `port/schema.Field`.
- [x] 3.3 Implement `Resolve` (REST mapper) and `Resources` (discovery `ServerPreferredResources`, same filtering as current `runShowTables`) mapping to `[]Resource`.
- [x] 3.4 Implement `InferSchema` (composite inferrer) and `List` (paginated dynamic LIST → `[]map[string]any` via `unstructured.Object`, invoking `pageFn` per page, with the existing per-page debug log).
- [x] 3.5 Add constructor `New(kubeconfig, kubeContext string) (k8sport.DataSource, error)` as the single wiring entry point.

## 4. Rewire consumers

- [x] 4.1 `internal/executor`: replace `dynamic.Interface`/mapper/inferrer fields with a `k8sport.DataSource`; `Run` calls `ds.List(..., pageFn)` and maps `map[string]any` pages to octosql values via the existing `resolveFieldValue`. Remove all `k8s.io/*` imports from this package.
- [x] 4.2 `cmd/root.go`: build the `DataSource` via the adapter `New(...)` in the pipeline; route `SHOW TABLES` through `Resources`, `DESCRIBE TABLE` through `Resolve`+`InferSchema`, and pass the port to the executor. Remove direct client-go usage from `cmd` except the adapter constructor.
- [x] 4.3 `internal/repl/complete.go` / `cmd` completion source: back `Tables()`/`Columns()` with the port (`Resources`/`Resolve`+`InferSchema`).

## 5. Remove old packages

- [x] 5.1 Delete `internal/k8s/` once nothing imports it.
- [x] 5.2 Delete the now-empty/moved files from `internal/schema/` (or remove the package if fully relocated); update all imports.

## 6. Boundary + regression

- [x] 6.1 Add `internal/adapter/datasources/k8s/boundary_test.go`: scan the tree; assert `k8s.io/client-go`, `k8s.io/apimachinery`, `k8s.io/kube-openapi`, `k8s.io/client-go/discovery` are imported only under `internal/adapter/datasources/k8s` and `cmd`.
- [x] 6.2 Run `make lint build`; fix issues.
- [x] 6.3 Run unit tests and the envtest integration suite — all existing scenarios MUST pass unchanged (no behavior change).
