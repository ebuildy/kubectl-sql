## MODIFIED Requirements

### Requirement: Engine owns the full query pipeline and rendering
The octosql adapter SHALL own the complete pipeline: table-qualifier rewriting (bare `FROM <resource>` becomes `FROM k8s.<resource>`), parsing, logical and physical planning, typecheck, optimization, ORDER BY and LIMIT handling, execution against the injected data source, and rendering of result rows to table, JSON, and CSV. Consumers SHALL NOT perform any of these steps.

#### Scenario: Output formats are produced by the engine
- **WHEN** `Execute` is called with output format `table`, `json`, or `csv`
- **THEN** the engine renders the result in that format, matching the pre-refactor output byte-for-byte

#### Scenario: Arrow field paths and helper functions resolve nested data without a dot rewrite
- **WHEN** a query uses arrow notation (`metadata->labels->app`) or helper functions (`map_get`, `map_contains_key`, `map_values`, `keys`, `contains`, `array_get`)
- **THEN** the engine resolves it directly, with no dot-notation rewrite step
