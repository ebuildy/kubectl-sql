## 1. Kubernetes Client Bootstrap

- [x] 1.1 Implement `NewDynamicClient(kubeconfig, context string) (dynamic.Interface, meta.RESTMapper, error)` in `internal/k8s/client.go` using `clientcmd.BuildConfigFromFlags`
- [x] 1.2 Wire REST mapper using `restmapper.GetAPIGroupResources` + `restmapper.NewDiscoveryRESTMapper`
- [x] 1.3 Write unit test for `NewDynamicClient` with an invalid kubeconfig path (expects error)

## 2. Field Resolver

- [x] 2.1 Create `internal/executor/resolver.go` with `ResolveField(obj map[string]interface{}, path string) interface{}` — supports dot notation (`status.phase`) and bracket notation (`.metadata.labels['app']`)
- [x] 2.2 Return `nil` for any path that does not exist (no error)
- [x] 2.3 Write unit tests covering: top-level field, nested field, missing field (returns nil), bracket label access

## 3. octosql Datasource

- [x] 3.1 Define `KubernetesDatasource` struct in `internal/executor/executor.go` holding `dynamic.Interface`, `meta.RESTMapper`, namespace string, and page size int
- [x] 3.2 Implement resource kind resolution: use REST mapper to convert FROM clause table name to `schema.GroupVersionResource` — accept plural, singular, and short names
- [x] 3.3 Implement paginated LIST: loop `dynamicClient.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{Limit: pageSize, Continue: token})` until `Continue` is empty
- [x] 3.4 Convert each `unstructured.Unstructured` item to an octosql `Object` value by passing the raw `.Object` map
- [x] 3.5 Implement the octosql `datasources.DataSourceRepository` registration so octosql can look up the datasource by table name at query time
- [x] 3.6 Write unit test for resource resolution (mock REST mapper): plural → GVR, short name → GVR, unknown → error

## 4. Command Wiring

- [x] 4.1 In `cmd/root.go`, change `RunE` to accept `Args: cobra.MaximumNArgs(1)` and detect when a SQL argument is present
- [x] 4.2 When a SQL argument is provided: read all flag values (`--kubeconfig`, `--context`, `--namespace`, `--page-size`, `--timeout`), build the dynamic client, register the datasource, and call octosql to execute the query
- [x] 4.3 When no SQL argument is provided: keep existing `cmd.Help()` behaviour
- [x] 4.4 On octosql parse/execution error: print error to stderr and return exit code 1
- [x] 4.5 Apply `--timeout` as a context deadline wrapping the entire query execution

## 5. Table Output

- [x] 5.1 Wire octosql's built-in table output to `os.Stdout` — use whatever output method octosql v0.13 exposes for printing a result set as a table
- [x] 5.2 Verify column names match the SELECT field list (or `*` for all fields)

## 6. End-to-End Test

- [x] 6.1 Add a godog step definition that stubs a kubeconfig pointing to a fake/mock cluster, or skip cluster-dependent scenarios with a `KUBECONFIG` env guard
- [x] 6.2 Add `test/e2e/features/sql.feature` with scenario: running with an invalid SQL string exits 1 and prints an error (no cluster needed)
- [x] 6.3 Add scenario to `help.feature`: running with `--help` still exits 0 after the positional arg change

## 7. Verification

- [x] 7.1 Run `go build ./...` — exits 0
- [x] 7.2 Run `make lint` — exits 0
- [x] 7.3 Run `make test` — exits 0, all unit tests pass
- [x] 7.4 Run `make e2e` — help and invalid-SQL scenarios pass
- [ ] 7.5 Manual smoke test: `make build && ./bin/kubectl-sql "SELECT name FROM pods"` against a real cluster (or `kind`)
