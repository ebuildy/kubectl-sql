# Spec: Logging

## Purpose

Defines leveled logging for `kubectl-sql`: a `-v`/`-vv` verbosity flag, stderr-only output so query results stay clean, a logger shared through the command context, isolation of the logging library behind a port/adapter boundary, and useful debug/info traces (with timings) at key execution boundaries.

---

## Requirements

### Requirement: Verbosity flag controls log level
The CLI SHALL accept a repeatable `-v` / `--verbose` flag that sets the logging level. With no flag the level SHALL be `error`; `-v` SHALL set `info`; `-vv` (or more) SHALL set `debug`.

#### Scenario: Default level is error
- **WHEN** the user runs a query with no `-v` flag
- **THEN** only `error`-level messages are emitted; `info` and `debug` messages are suppressed

#### Scenario: -v enables info
- **WHEN** the user runs a query with `-v`
- **THEN** `info` and `error` messages are emitted; `debug` is suppressed

#### Scenario: -vv enables debug
- **WHEN** the user runs a query with `-vv`
- **THEN** `debug`, `info`, and `error` messages are all emitted

### Requirement: Logs are written to stderr
All log output SHALL be written to stderr so that query results on stdout remain clean and machine-parseable.

#### Scenario: Query output on stdout is unaffected by logging
- **WHEN** the user runs `kubectl-sql -vv --output json "SELECT name FROM pods"` and captures stdout separately from stderr
- **THEN** stdout contains only the valid JSON result and all log lines appear on stderr

### Requirement: Logger is available throughout the execution pipeline
A leveled logger SHALL be constructed once per invocation and made available to the Kubernetes data source, schema inference, query engine, REPL, and watch components via the command context. Components SHALL retrieve it from context rather than constructing their own.

#### Scenario: Components log through the shared logger
- **WHEN** any pipeline component emits a log entry
- **THEN** the entry is produced by the single context logger and respects the configured level

#### Scenario: Missing logger degrades safely
- **WHEN** a component requests the logger from a context that has none
- **THEN** a no-op logger is returned and the component does not panic

### Requirement: Logging library is isolated behind a port
The logging implementation SHALL be hidden behind a domain-owned `Logger` interface in a dedicated port package (`internal/port/logger`). The concrete binding SHALL live in a separate adapter package (`internal/adapter/logger/zap`). All application code SHALL depend only on the port — its interface and field constructors — never on the underlying logging library directly. Only the adapter package, plus the `cmd` composition root that wires it, may reference the library.

#### Scenario: Only the adapter (and wiring) imports the logging library
- **WHEN** the source tree is scanned for imports of `go.uber.org/zap`
- **THEN** the import appears only in the adapter package `internal/adapter/logger/zap` and the `cmd` composition root, and nowhere else

#### Scenario: Library can be swapped without touching call sites
- **WHEN** the zap adapter is replaced by a sibling adapter package backed by a different library that satisfies the `Logger` interface
- **THEN** no package outside `internal/adapter/logger/*` and the one `cmd` wiring line requires modification

### Requirement: Useful debug and info traces at key boundaries
The system SHALL emit `info` logs for major lifecycle events (cluster connection established, query accepted, REPL/watch started) and `debug` logs for detailed steps (resolved resource, schema inference source, parsed/typechecked/optimized plan, LIST pagination, per-tick watch refresh). Debug logs SHALL include elapsed time in milliseconds for the total query, schema inference, and each LIST page, via a library-agnostic duration field constructor.

#### Scenario: Info trace on query execution
- **WHEN** the user runs a query with `-v`
- **THEN** an `info` log records that the query was accepted and the target resource

#### Scenario: Debug trace on schema inference
- **WHEN** the user runs a query with `-vv`
- **THEN** a `debug` log records which inferrer (OpenAPI or sample) supplied the schema and the resolved resource

#### Scenario: Debug traces record timing
- **WHEN** the user runs a query with `-vv`
- **THEN** debug logs include an elapsed-milliseconds field for the total query and for schema inference
