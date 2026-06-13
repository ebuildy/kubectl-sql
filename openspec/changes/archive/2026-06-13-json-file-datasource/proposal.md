## Why

octosql, the SQL engine kubectl-sql is built on, ships a native JSON Lines file
datasource (`github.com/cube2222/octosql/datasources/json`) that infers a schema by
sampling a local file and streams each line as a row. kubectl-sql's adapter registers
an empty `FileHandlers` map
(`internal/adapter/sql/octosql/engine.go`), so `SELECT ... FROM data.json` currently
fails with "no such table: data.json". octosql's table-name routing
(`physical.DatasourceRepository.GetDatasource`) and kubectl-sql's existing
table-qualifier rewrite (`FROM pods` → `FROM k8s.pods`) already cooperate correctly
for dotted file-path table names — a `FROM <name>.json` reference parses with a
non-empty `Qualifier`, so the `k8s.` rewrite is skipped and the name falls through to
extension-based file-handler routing untouched. Wiring up the JSON file handler is
the missing piece that turns local JSON Lines files into a first-class queryable
source, registered alongside the existing `k8s` database in the same
`physical.DatasourceRepository` so both kinds of tables can be referenced without
"no such table" errors — including `JOIN`s between multiple JSON file tables (e.g.
`SELECT n.pod, n.note, s.status FROM notes.json n JOIN status.json s ON n.pod =
s.pod`).

> **Note on mixing with Kubernetes tables:** while investigating this change we found
> that `JOIN` execution against a `k8s.*`-routed table currently returns no/incorrect
> rows regardless of the other side of the join (even `pods p JOIN pods p2 ON
> p.name = p2.name` returns null-filled rows) — a pre-existing issue in
> `internal/adapter/sql/octosql/database.go`'s `KubernetesDatabase` execution, unrelated
> to JSON files. `JOIN`ing a `k8s.*` table with a `*.json` table is therefore **out of
> scope for this change** and tracked as a follow-up once that execution bug is fixed.
> See `design.md` Non-Goals.

## What Changes

- Register octosql's `datasources/json.Creator` as the file handler for the `json`,
  `jsonl`, and `ndjson` extensions in the `physical.DatasourceRepository.FileHandlers`
  map built in `internal/adapter/sql/octosql/engine.go`. All three extensions map to
  the same JSON Lines reader (one JSON object per line).
- Inject a default `*github.com/cube2222/octosql/config.Config` (file buffer-size
  defaults) into the query `context.Context` via `config.ContextWithConfig` before
  the pipeline runs, since octosql's file-reading code
  (`execution/files`, `datasources/json`) calls `config.FromContext(ctx)` and panics
  if no config has been set.
- Fix `typecheckNode` in `internal/adapter/sql/octosql/engine.go`: its panic-recovery
  was commented out, so any `physical.DatasourceRepository.GetDatasource` error
  (including "no such table" and, with this change, a non-JSON-Lines file passed to
  `json.Creator`) crashed the whole process instead of returning a clean error.
  Enable it, mirroring the identical pattern already active in `typecheckExpr`.
- Add unit tests proving: `SELECT * FROM <file>.json` (and `.jsonl`/`.ndjson`) reads a
  local JSON Lines file with inferred columns; WHERE/column selection on JSON fields
  works; and a single query can `JOIN` two JSON file tables together.
- Add an e2e/integration scenario with a JSON Lines fixture file under
  `test/fixtures/`.
- Document the new `FROM <file>.json` source (README, AGENTS.md SQL grammar
  reference) including the JSON Lines (one object per line) requirement and the
  `./` escape hatch for a file literally named `k8s.json`/`k8s.jsonl`/`k8s.ndjson`
  (which would otherwise collide with the registered `k8s` database name).

## Capabilities

### New Capabilities
- `json-file-datasource`: Querying local JSON Lines files via `FROM <path>.json`
  (or `.jsonl`/`.ndjson`), including schema inference, column selection, filtering,
  ordering/limiting, and combining multiple JSON file tables via `JOIN` in a single
  query.

### Modified Capabilities
- `sql-engine-port`: The octosql adapter's `physical.Environment.Datasources` now
  populates `FileHandlers` (previously empty) and the pipeline injects an octosql
  `config.Config` into context — both part of "the engine owns the full query
  pipeline". `typecheckNode` now recovers from typecheck-time panics (e.g. datasource
  creation errors) and returns them as errors instead of crashing.

## Impact

- Code: `internal/adapter/sql/octosql/engine.go` (FileHandlers registration, config
  context injection, `typecheckNode` panic recovery).
- Dependencies: new transitive imports from `github.com/cube2222/octosql/datasources/json`
  and `github.com/cube2222/octosql/config` (already part of the existing
  `github.com/cube2222/octosql v0.13.0` requirement, no version bump) — see
  `design.md` for the full transitive dependency list.
- Tests: new unit tests in `internal/adapter/sql/octosql/`, new e2e feature +
  fixture file under `test/fixtures/`.
- Docs: `README.md`, `AGENTS.md`.
