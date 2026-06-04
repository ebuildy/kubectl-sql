## Why

The project scaffold exists but the tool does nothing useful yet — running `kubectl sql "SELECT ..."` produces only help text. This change delivers the core feature: executing a SQL query against live Kubernetes resources and printing the results as a table.

## What Changes

- Accept the SQL query string as the first positional argument of the root command
- Implement a `client-go` dynamic client bootstrap in `internal/k8s/` (kubeconfig, context, namespace flags wired)
- Implement a Kubernetes datasource in `internal/executor/` that implements the octosql `DataSource` interface — lists resources via the dynamic client using REST mapper discovery, streams rows as octosql `Object` values
- Wire octosql query execution in `cmd/root.go`: parse SQL → resolve datasource → execute → stream results
- Render the result set as a plain table using octosql's built-in table output (stdout)
- Propagate `--namespace`, `--context`, `--kubeconfig`, `--page-size`, `--timeout` flags into the k8s client and executor

## Capabilities

### New Capabilities

- `sql-execution`: Accept a SQL query argument, execute it against Kubernetes via octosql + client-go, and print results as a table
- `k8s-datasource`: octosql `DataSource` implementation backed by the client-go dynamic client — maps FROM clause resource kinds to k8s LIST calls, exposes each resource's unstructured JSON fields as row columns

### Modified Capabilities

- `project-scaffold`: Root command now accepts a positional SQL argument and runs a query instead of printing help when one is provided

## Impact

- `cmd/root.go` — adds positional arg handling and query execution path
- `internal/k8s/client.go` — implements kubeconfig bootstrap, exports `NewDynamicClient`
- `internal/executor/executor.go` — implements octosql datasource adapter
- `internal/executor/resolver.go` — JSON path field resolution on unstructured objects (new file)
- No new `go.mod` dependencies — octosql and client-go already present
