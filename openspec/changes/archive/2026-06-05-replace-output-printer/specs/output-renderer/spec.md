## ADDED Requirements

### Requirement: Output renderer collects and renders octosql records without TTY dependency
The `internal/output` package SHALL provide a `Render` function that drives an octosql `execution.Node`, collects all records, applies ORDER BY and LIMIT, and writes results to an `io.Writer` with no dependency on terminal state or `/dev/tty`.

#### Scenario: Table output renders to stdout
- **WHEN** `Render` is called with format `table` and a node that produces rows
- **THEN** a formatted table is written to the writer and the function returns nil

#### Scenario: Process exits cleanly after render
- **WHEN** `kubectl-sql "SELECT name FROM pods"` is run in any shell environment (TTY or not)
- **THEN** the process prints results and exits 0 without hanging

#### Scenario: COUNT(*) exits cleanly
- **WHEN** `kubectl-sql "SELECT COUNT(*) FROM pods"` is run
- **THEN** the process prints the count and exits 0 without hanging

### Requirement: Output format is controlled by --output flag
The renderer SHALL support `table` (default), `json`, and `csv` output formats selected via `--output/-o`.

#### Scenario: Default format is table
- **WHEN** `kubectl-sql "SELECT name FROM pods"` is run without `--output`
- **THEN** output is a formatted ASCII table

#### Scenario: JSON format outputs JSON array
- **WHEN** `kubectl-sql --output json "SELECT name FROM pods"` is run
- **THEN** output is a JSON array of objects, one per row

#### Scenario: CSV format outputs comma-separated values
- **WHEN** `kubectl-sql --output csv "SELECT name FROM pods"` is run
- **THEN** output is CSV with a header row
