## MODIFIED Requirements

### Requirement: Engine owns the full query pipeline and rendering
The octosql adapter SHALL own the complete pipeline: table-qualifier rewriting (bare
`FROM <resource>` becomes `FROM k8s.<resource>`), parsing, logical and physical
planning, typecheck, optimization, ORDER BY and LIMIT handling, execution against the
injected data source, and rendering of result rows to table, JSON, and CSV.
Consumers SHALL NOT perform any of these steps.

In addition to the `k8s` database, the adapter's `physical.DatasourceRepository`
SHALL register file-based datasource handlers for local JSON Lines files (`.json`,
`.jsonl`, `.ndjson` extensions), so a `FROM` reference with one of these extensions
and a non-empty table-name qualifier (left untouched by the `k8s.` table-qualifier
rewrite, since the rewrite only applies to unqualified bare names) resolves to a
local file rather than a Kubernetes resource. Before running the pipeline, the
adapter SHALL inject an octosql `config.Config` into the query's `context.Context`
(via `config.ContextWithConfig`), since octosql's file-based datasource
implementations read configuration from the context and otherwise panic.

#### Scenario: Output formats are produced by the engine
- **WHEN** `Execute` is called with output format `table`, `json`, or `csv`
- **THEN** the engine renders the result in that format, matching the pre-refactor
  output byte-for-byte

#### Scenario: Arrow field paths and helper functions resolve nested data without a dot rewrite
- **WHEN** a query uses arrow notation (`metadata->labels->app`) or helper functions
  (`map_get`, `map_contains_key`, `map_values`, `keys`, `contains`, `array_get`)
- **THEN** the engine resolves it directly, with no dot-notation rewrite step

#### Scenario: A FROM reference with a .json/.jsonl/.ndjson extension routes to the file handler
- **WHEN** `Execute` is called with a query containing `FROM <name>.json` (or
  `.jsonl`/`.ndjson`), where `<name>` is non-empty
- **THEN** the table-qualifier rewrite leaves the reference unchanged (its qualifier
  is already non-empty), and `physical.DatasourceRepository.GetDatasource` resolves
  it via the registered JSON file handler rather than the `k8s` database

#### Scenario: Query context carries an octosql config before planning
- **WHEN** `Execute` runs the pipeline for any query
- **THEN** the `context.Context` passed to parsing, typechecking, and execution
  carries an octosql `config.Config` (via `config.ContextWithConfig`), so that
  `config.FromContext` does not panic when a file-based datasource is resolved
