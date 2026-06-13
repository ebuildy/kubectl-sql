## 1. Engine wiring

- [x] 1.1 In `internal/adapter/sql/octosql/engine.go`, import
      `github.com/cube2222/octosql/datasources/json` and
      `github.com/cube2222/octosql/config`.
- [x] 1.2 Populate `physical.DatasourceRepository.FileHandlers` with `"json"`,
      `"jsonl"`, and `"ndjson"` keys, all mapped to `json.Creator`.
- [x] 1.3 At the start of `Execute`, wrap the incoming `context.Context` with
      `config.ContextWithConfig(ctx, &config.Config{...})` using the hardcoded
      defaults from design.md (`Files.BufferSizeBytes = 4*1024*1024`,
      `Files.JSON.MaxLineSizeBytes = 1024*1024`), before `rewriteQuery` /
      `sqlparser.Parse` / `typecheckNode` run.
- [x] 1.4 Run `go mod tidy` and review the `go.mod`/`go.sum` diff against the
      dependency list in design.md (`github.com/nxadm/tail`,
      `github.com/adrg/xdg`, `github.com/mitchellh/go-homedir`,
      `github.com/Masterminds/semver`, `github.com/valyala/fastjson` promoted to
      direct).
- [x] 1.5 Enable the commented-out panic recovery in `typecheckNode`
      (`internal/adapter/sql/octosql/engine.go`), mirroring the identical pattern
      already active in `typecheckExpr`, so `GetDatasource` errors (e.g. "no such
      table", or `json.Creator` rejecting a non-JSON-Lines file) become clean
      errors instead of process panics. See design.md "Enable typecheckNode's
      panic recovery".

## 2. Unit tests

- [x] 2.1 Add `internal/adapter/sql/octosql/json_datasource_test.go`. Using a
      `t.TempDir()` fixture file containing JSON Lines rows (e.g.
      `{"pod":"nginx-1","note":"ok"}`), assert
      `SELECT * FROM <path>.json` via `eng.Execute` returns one row per line with
      columns matching the JSON keys (follow the `New(portsql.Config{Output:
      "json"}, fakeDS)` / JSON-output-assertion pattern from
      `length_query_test.go`).
- [x] 2.2 In the same file, add a test for `.jsonl` and `.ndjson` extensions
      against the same fixture content, asserting identical output.
- [x] 2.3 Add a test for column selection and `WHERE` on JSON file fields
      (`SELECT pod, note FROM <path>.json WHERE pod = 'nginx-1'`), and a test for
      `ORDER BY` + `LIMIT` with `-o json` output.
- [x] 2.4 Add a JOIN test: two JSON Lines fixture files (e.g. `notes.json` and
      `status.json`, sharing a `pod` key) `JOIN`ed in a single query
      (`SELECT n.pod, n.note, s.status FROM notes.json n JOIN status.json s ON
      n.pod = s.pod`), assert the joined rows match (out of scope: joining against
      a `k8s`-routed table — see design.md Non-Goals).
- [x] 2.5 Add a negative test: a fixture file containing a single pretty-printed
      top-level JSON array (not JSON Lines) referenced via `FROM <path>.json`
      returns a non-nil error from `Execute` whose message indicates a line could
      not be parsed as a JSON object.

## 3. End-to-end test

- [x] 3.1 Add `test/fixtures/notes.jsonl` containing a few JSON-Lines rows (e.g.
      `{"pod":"nginx-1","note":"ok"}` / `{"pod":"nginx-2","note":"check logs"}`).
- [x] 3.2 Add `test/e2e/features/json-datasource.feature` with a scenario using the
      existing envtest-gated step (`When I run kubectl-sql "<query>" against the
      envtest cluster`, matching the pattern in `integration.feature` /
      `steps_test.go`'s `iRunKubectlSqlAgainstEnvtest`) — this provides the
      loadable kubeconfig that `NewQueryCommand` requires even for a
      file-only query:
      - `SELECT * FROM ../fixtures/notes.jsonl` (relative to the `test/e2e`
        package's working directory) returns the fixture rows. `test/e2e`'s
        `steps_test.go` does not register the `the output produces JQ` step used
        by `integration.feature` (that step lives only in `test/integration`'s
        envtest-backed suite), so this scenario asserts via the steps that ARE
        registered for `test/e2e`: `the output has between 2 and 2 rows` and
        `the output contains "..."` for each fixture value (`nginx-1`,
        `nginx-2`, `check logs`).
- [x] 3.3 Confirm the scenario is skipped (not failed) when `ENVTEST_KUBECONFIG` is
      unset, consistent with `integration.feature` (both use
      `iRunKubectlSqlAgainstEnvtest`, which returns `godog.ErrSkip` when
      `ENVTEST_KUBECONFIG` is unset). Verified in task 5.3.

## 4. Documentation

- [x] 4.1 Update `README.md` to document the new `FROM <path>.json` (and
      `.jsonl`/`.ndjson`) source: JSON Lines format requirement, schema
      inference, relative/absolute path syntax, the `./k8s.json` escape hatch for a
      file named `k8s.json`, the `<table>.<field>` column-prefix convention (and
      the `jsonl`/`ndjson` extension-vs-basename quirk — recommend `AS <alias>`),
      and a `JOIN` between two JSON file tables. Note that joining a `k8s.*` table
      with a `*.json` table is not yet supported (tracked as a follow-up).
- [x] 4.2 Update `AGENTS.md`'s SQL grammar reference / debug recipes with an
      example query against a `.json`/`.jsonl` file, including a JSON-JSON `JOIN`
      example.

## 5. Verify

- [x] 5.1 Run `make lint build` and fix any issues.
- [x] 5.2 Run `go test ./... -race -count=1` and confirm all new and existing
      tests pass, including `TestOctosqlImportBoundary`.
- [x] 5.3 Run the e2e suite with `ENVTEST_KUBECONFIG` set and confirm the new
      `json-datasource.feature` scenarios pass; run without it set and confirm
      they are skipped cleanly. Verified against a local envtest cluster: with
      `ENVTEST_KUBECONFIG` set, `SELECT * FROM test/fixtures/notes.jsonl` passes
      (exit 0, 2 rows, fixture values present); without it, the scenario is
      skipped and the suite still exits 0.

      Note: along the way, discovered that octosql's lexer does not support
      `../`-prefixed (parent-relative) table-name paths (a leading bare `.`
      token is a syntax error — confirmed via
      `SELECT * FROM ../fixtures/notes.jsonl` failing with "invalid argument
      syntax error at position 16"). To let the feature reference
      `test/fixtures/notes.jsonl` (matching this task's wording and the
      `test/fixtures/` convention) without `..`, `test/e2e/steps_test.go`'s
      `runCommand` now sets `cmd.Dir` to the repo root (new `repoRoot()` helper,
      reused by `binaryPath()`) — a one-line test-harness change with no effect
      on existing scenarios, since none of them depend on the working
      directory.
