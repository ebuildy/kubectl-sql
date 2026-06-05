## 1. k8s Client

- [x] 1.1 Update `NewDynamicClient` in `internal/k8s/client.go` to return `discovery.DiscoveryInterface` as a fourth return value
- [x] 1.2 Update all callers of `NewDynamicClient` in `cmd/root.go` to accept the new return value

## 2. SHOW TABLES Handler

- [x] 2.1 Add `runShowTables(discoClient discovery.DiscoveryInterface) error` in `cmd/root.go` that calls `discoClient.ServerPreferredResources()`, iterates results, and prints a table with columns `NAME`, `GROUP`, `VERSION` using `tablewriter`
- [x] 2.2 Sort output by group then resource name for deterministic output
- [x] 2.3 In `runQuery`, detect `SHOW TABLES` (case-insensitive, trimmed) before `rewriteQuery` and call `runShowTables`

## 3. Unit Tests

- [x] 3.1 Add unit test in `internal/k8s/` verifying `NewDynamicClient` returns a non-nil discovery client
- [x] 3.2 Add unit test for the `SHOW TABLES` detection logic (case variants: `show tables`, `SHOW TABLES`, `  Show Tables  `)

## 4. e2e Feature

- [x] 4.1 Add scenario to `test/e2e/features/sql.feature`: `SHOW TABLES` with `--kubeconfig /nonexistent` exits non-zero (no cluster, but parses correctly before connect attempt)
- [x] 4.2 Add step definitions to `test/e2e/steps_test.go` if needed

## 5. Integration Feature

- [x] 5.1 Add scenario to `test/e2e/features/integration.feature`: `SHOW TABLES` against envtest cluster exits 0 and output contains `pods`
- [x] 5.2 Add step definition `the output contains {string}` if not already registered in `test/integration/steps_test.go` (already exists — verify)

## 6. Verification

- [x] 6.1 Run `go build ./...` — exits 0
- [x] 6.2 Run `make lint` — exits 0
- [x] 6.3 Run `make test` — exits 0
- [x] 6.4 Run `make e2e` — exits 0
- [x] 6.5 Run `make e2e-run-fake` — all scenarios pass
