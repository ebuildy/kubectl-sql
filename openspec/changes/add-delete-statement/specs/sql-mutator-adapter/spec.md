## ADDED Requirements

### Requirement: Mutating statements are handled by a dedicated mutator adapter
All mutating SQL statements (DELETE now; UPDATE later) SHALL be handled by a dedicated `mutator`
SQL adapter at `internal/adapter/sql/mutator`, a sibling of the `octosql` adapter under
`internal/adapter/sql`, because octosql supports only `SELECT`. The octosql adapter SHALL remain
SELECT-only and SHALL never be asked to parse a DELETE statement. The mutator adapter SHALL be the
only package containing mutating-statement (DELETE/UPDATE) logic.

#### Scenario: DELETE is handled by the mutator adapter, not octosql
- **WHEN** a `DELETE` statement is executed
- **THEN** it is dispatched to the `mutator` adapter, and the octosql adapter is never asked to parse it

#### Scenario: octosql adapter stays SELECT-only
- **WHEN** the `internal/adapter/sql/octosql` package is inspected
- **THEN** it contains no DELETE/UPDATE handling; mutating-statement logic lives only in `internal/adapter/sql/mutator`

### Requirement: Mutator delegates row resolution to the octosql SELECT engine
The mutator adapter SHALL resolve the set of objects a mutating statement affects by delegating to
the octosql SELECT engine: for `DELETE [FROM] <resource> [WHERE <expr>]` it SHALL run the
equivalent `SELECT namespace, name FROM <resource> [WHERE <expr>]` through the octosql adapter and
read back the resulting rows. The mutator SHALL NOT re-implement WHERE-clause evaluation; the
match set SHALL therefore be identical to what the equivalent SELECT would return.

#### Scenario: DELETE resolves targets via a delegated SELECT
- **WHEN** the mutator handles `DELETE FROM pod WHERE name = 'harbor'`
- **THEN** it executes `SELECT namespace, name FROM pod WHERE name = 'harbor'` through the octosql engine and uses the returned rows as the deletion set

#### Scenario: WHERE semantics match SELECT exactly
- **WHEN** a DELETE and the equivalent SELECT use the same WHERE expression
- **THEN** the mutator's deletion set equals the SELECT's result set, with no separately implemented filter logic

### Requirement: Mutator performs mutations through the DataSource port
The mutator adapter SHALL perform the actual mutation through the Kubernetes `DataSource` port
(e.g. `DataSource.Delete`), never by importing client-go directly. The mutator adapter SHALL be
injected with both the octosql SELECT engine and the `DataSource` port via its constructor, so it
holds no Kubernetes library dependency of its own.

#### Scenario: Mutator deletes via the DataSource port
- **WHEN** a resolved DELETE proceeds to execution
- **THEN** the mutator calls `DataSource.Delete` for each target and does not import any `k8s.io/*` package

### Requirement: Apply deletes with bounded parallelism and aggregates results
The mutator adapter's `Apply` SHALL delete the plan's targets concurrently with at most 10
in-flight deletions, collect each target's outcome (success or error) without data races, wait
for all to complete, and return a `DeleteResult` carrying every per-object outcome in the plan's
original order. `Apply` SHALL NOT print to the user — rendering is the caller's responsibility. To
support progress reporting, `Apply` SHALL accept an optional `onProgress func()` callback and
invoke it exactly once per completed delete; the callback MUST be safe to call concurrently.

#### Scenario: Apply bounds concurrency at 10
- **WHEN** `Apply` is given a plan with more than 10 targets
- **THEN** it runs deletions concurrently with no more than 10 in flight and returns only after all targets have been attempted

#### Scenario: Apply aggregates per-object outcomes
- **WHEN** some target deletions succeed and others fail
- **THEN** the returned `DeleteResult` records each target's success or failure (with the error) in the plan's order

#### Scenario: Apply reports progress per completed delete
- **WHEN** `Apply` is called with a non-nil `onProgress` callback over N targets
- **THEN** the callback is invoked exactly N times, once per completed delete, and the mutator package does not import the progress-bar library

#### Scenario: Mutator is wired from its dependencies
- **WHEN** the mutator adapter is constructed
- **THEN** it receives the octosql SELECT engine and the `DataSource` port as constructor arguments
