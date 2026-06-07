## ADDED Requirements

### Requirement: SQL engine is defined by a domain-typed port
The SQL engine SHALL be expressed as an `Engine` interface in `internal/port/sql`. No exported type or method signature on the port SHALL reference `github.com/cube2222/octosql` or any of its subpackages. The query and its options SHALL be expressed in plain Go (a `Query` struct with SQL text, output format, namespace, page size, and color preference) and the result SHALL be written to an `io.Writer`.

#### Scenario: Port signatures are library-free
- **WHEN** the `internal/port/sql` package is compiled
- **THEN** it does not import any `github.com/cube2222/octosql` package, and its interface uses only standard-library and domain types

#### Scenario: Execute writes rendered output
- **WHEN** a consumer calls `Engine.Execute(ctx, Query{SQL: "SELECT name FROM pods", Output: "json"}, w)`
- **THEN** the rendered JSON result is written to `w` and the call returns nil on success

### Requirement: octosql is confined to the adapter package
octosql SHALL be imported only by the adapter package `internal/adapter/sql/octosql`. No other package SHALL import `github.com/cube2222/octosql` or its subpackages.

#### Scenario: Only the adapter imports octosql
- **WHEN** the source tree is scanned for imports of `github.com/cube2222/octosql`
- **THEN** those imports appear only within `internal/adapter/sql/octosql`

#### Scenario: Engine can be swapped without touching consumers
- **WHEN** the octosql adapter is replaced by another adapter satisfying the `Engine` port
- **THEN** no package outside `internal/adapter/sql/*` and the `cmd` wiring requires modification

### Requirement: Engine owns the full query pipeline and rendering
The octosql adapter SHALL own the complete pipeline: dot/arrow query rewriting, parsing, logical and physical planning, typecheck, optimization, ORDER BY and LIMIT handling, execution against the injected data source, and rendering of result rows to table, JSON, and CSV. Consumers SHALL NOT perform any of these steps.

#### Scenario: Output formats are produced by the engine
- **WHEN** `Execute` is called with output format `table`, `json`, or `csv`
- **THEN** the engine renders the result in that format, matching the pre-refactor output byte-for-byte

#### Scenario: Dot and arrow field paths still work
- **WHEN** a query uses dot notation (`metadata.labels.app`) or arrow notation (`metadata->labels->app`)
- **THEN** the engine rewrites and resolves it exactly as before the refactor

### Requirement: Engine consumes the Kubernetes data source via its port
The octosql engine SHALL obtain cluster rows and schema through the injected `internal/port/datasources/k8s` `DataSource` port. The adapter SHALL NOT import `k8s.io/client-go`, `k8s.io/apimachinery`, or other Kubernetes libraries.

#### Scenario: Engine is constructed with a data source
- **WHEN** the engine is built via `octosql.New(ds)` where `ds` is a k8s `DataSource`
- **THEN** queries resolve tables, infer schema, and stream rows through that port

#### Scenario: SQL adapter does not import client-go
- **WHEN** the `internal/adapter/sql/octosql` package is scanned for imports
- **THEN** it imports no `k8s.io/*` package (cluster access is solely via the k8s port)

### Requirement: Existing query behavior is preserved
Routing queries through the engine — including SELECT, WHERE, ORDER BY, LIMIT, GROUP BY/aggregates, DISTINCT, struct columns, the REPL, and watch — SHALL produce the same results, exit codes, and output formats as before the refactor.

#### Scenario: Query output is unchanged
- **WHEN** any previously-passing query is run after the refactor
- **THEN** its output and exit code match the pre-refactor behavior (the existing integration suite passes unchanged)
