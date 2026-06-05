## Why

Users have no way to discover which Kubernetes resource types are queryable — they must know the resource name in advance. `SHOW TABLES` gives a familiar SQL introspection command that lists all available resources, making the tool self-documenting.

## What Changes

- Add `SHOW TABLES` as a special query handled before the octosql pipeline
- Returns all Kubernetes API resource types (kind, group, version, namespaced flag) discoverable via the REST mapper
- Output formatted as a table like any other query result
- Works with `--context`, `--kubeconfig`, `--namespace` flags

## Capabilities

### New Capabilities

- `show-tables`: `SHOW TABLES` command that lists all queryable Kubernetes resource types

### Modified Capabilities

- `sql-execution`: New query entry point before SQL parsing — `SHOW TABLES` is intercepted and handled without going through octosql

## Impact

- `cmd/root.go` — detect `SHOW TABLES` before `rewriteQuery` / octosql pipeline
- `internal/executor/executor.go` — new `ListTables` implementation on `KubernetesDatabase`
- `test/e2e/features/sql.feature` — new scenario for `SHOW TABLES`
- `test/integration/` — new integration scenario
