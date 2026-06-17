## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: octosql is confined to the adapter package
octosql SHALL be imported only by the adapter package `internal/adapter/sql/octosql`. No other package SHALL import `github.com/cube2222/octosql` or its subpackages.

#### Scenario: Only the adapter imports octosql
- **WHEN** the source tree is scanned for imports of `github.com/cube2222/octosql`
- **THEN** those imports appear only within `internal/adapter/sql/octosql`

#### Scenario: Engine can be swapped without touching consumers
- **WHEN** the octosql adapter is replaced by another adapter satisfying the `Engine` and `EngineFactory` ports
- **THEN** no package outside `internal/adapter/sql/*` and the `internal/app` wiring requires modification
