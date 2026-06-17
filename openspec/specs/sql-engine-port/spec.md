# Spec: SQL Engine Port

## Purpose

Defines the hexagonal boundary between `kubectl-sql` and the SQL engine library (octosql). The engine is expressed as a domain-typed `Engine` port (`internal/port/sql`); all octosql code — the query pipeline (rewrite, parse, plan, typecheck, optimize, execute) and result rendering — is confined to a single adapter (`internal/adapter/sql/octosql`). The engine obtains cluster data through the Kubernetes `DataSource` port. These requirements govern the structural contract that keeps the engine swappable and octosql out of the rest of the codebase. Observable query behavior is governed by the `sql-execution`, `output-renderer`, and `k8s-datasource` specs.

---

## Requirements

### Requirement: SQL engine is defined by a domain-typed port
The SQL engine SHALL be expressed as an `Engine` interface in `internal/port/sql`. No exported type or method signature on the port SHALL reference `github.com/cube2222/octosql` or any of its subpackages. The query and its options SHALL be expressed in plain Go (a `Query` struct with SQL text, output format, namespace, page size, and color preference) and the result SHALL be written to an `io.Writer`.

#### Scenario: Port signatures are library-free
- **WHEN** the `internal/port/sql` package is compiled
- **THEN** it does not import any `github.com/cube2222/octosql` package, and its interface uses only standard-library and domain types

#### Scenario: Execute writes rendered output
- **WHEN** a consumer calls `Engine.Execute(ctx, Query{SQL: "SELECT name FROM pods", Output: "json"}, w)`
- **THEN** the rendered JSON result is written to `w` and the call returns nil on success

### Requirement: SQL engines are obtained through an EngineFactory port
The `internal/port/sql` package SHALL define an `EngineFactory` interface with a single method `New(cfg Config) Engine` that returns an `Engine` configured for the given `Config`. No exported type or method on `EngineFactory` SHALL reference `github.com/cube2222/octosql` or any of its subpackages. The octosql adapter SHALL provide a constructor `NewFactory(ds, sc)` (closing over the injected `DataSource` and `SpellChecker`) that returns an `EngineFactory` whose `New(cfg)` builds an octosql `Engine`. Consumers that need an engine SHALL obtain it from an injected `EngineFactory`, not by calling the octosql adapter directly.

#### Scenario: Factory port is library-free
- **WHEN** the `internal/port/sql` package is compiled
- **THEN** the `EngineFactory` interface uses only standard-library and domain types and imports no `github.com/cube2222/octosql` package

#### Scenario: Factory produces a configured engine
- **WHEN** a consumer holds an injected `EngineFactory` and calls `factory.New(Config{Output: "json"})`
- **THEN** it receives an `Engine` that, on `Execute`, renders results in JSON using the factory's data source and spell checker

#### Scenario: Consumers build engines via the factory
- **WHEN** the `internal/domain/...` packages are scanned for engine construction
- **THEN** they obtain engines only through the injected `EngineFactory` and contain no call to `octosql.New` or `octosql.NewFactory`

### Requirement: octosql is confined to the adapter package
octosql SHALL be imported only by the adapter package `internal/adapter/sql/octosql`. No other package SHALL import `github.com/cube2222/octosql` or its subpackages.

#### Scenario: Only the adapter imports octosql
- **WHEN** the source tree is scanned for imports of `github.com/cube2222/octosql`
- **THEN** those imports appear only within `internal/adapter/sql/octosql`

#### Scenario: Engine can be swapped without touching consumers
- **WHEN** the octosql adapter is replaced by another adapter satisfying the `Engine` and `EngineFactory` ports
- **THEN** no package outside `internal/adapter/sql/*` and the `internal/app` wiring requires modification

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
