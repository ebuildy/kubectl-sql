## ADDED Requirements

### Requirement: Struct values render as JSON in table and CSV output
When a result cell holds a struct-typed value, the renderer SHALL convert it to a JSON object whose keys are the struct field names resolved from the schema type. List and tuple values SHALL render as JSON arrays, and map columns (carried as flat key/value lists) SHALL render as JSON objects, matching what `--output json` produces for the same cell. Table output SHALL pretty-print the JSON with 2-space indentation; CSV output SHALL emit compact single-line JSON inside a properly quoted CSV field. Scalar values SHALL render exactly as before. If JSON conversion fails, the renderer SHALL fall back to the octosql string form rather than returning an error.

#### Scenario: Struct cell in table output is pretty JSON
- **WHEN** `kubectl-sql "SELECT name, status FROM pods"` is run with table output and `status` is a struct column
- **THEN** the `status` cell contains indented JSON with field names as keys (e.g. `{"phase": "Running", ...}` across multiple lines), not octosql's positional `{ v1, v2 }` form

#### Scenario: Struct cell in CSV output is compact JSON
- **WHEN** `kubectl-sql --output csv "SELECT name, status FROM pods"` is run and `status` is a struct column
- **THEN** the `status` field contains single-line compact JSON, the record remains one CSV line, and the output parses with a standard CSV reader

#### Scenario: Nested struct fields are resolved recursively
- **WHEN** a struct column contains a nested struct field
- **THEN** the nested value is rendered as a nested JSON object with its own field names

#### Scenario: List and tuple cells render as JSON arrays
- **WHEN** a query selects a list column (e.g. `spec.containers`) or produces a tuple value with table output
- **THEN** the cell contains a pretty-printed JSON array, with JSON-string elements decoded to objects

#### Scenario: Map cells render as JSON objects
- **WHEN** a query selects a map column (e.g. `metadata.labels`) with table output
- **THEN** the cell contains a pretty-printed JSON object keyed by the map keys, not the flat alternating key/value list

#### Scenario: Scalar cells are unchanged
- **WHEN** a query selects only scalar columns (string, int, float, boolean, time, null)
- **THEN** table and CSV cell rendering is byte-identical to the previous behavior

#### Scenario: JSON output format is unaffected
- **WHEN** `kubectl-sql --output json "SELECT status FROM pods"` is run
- **THEN** the output is identical to the behavior before this change

### Requirement: Pretty struct rendering can be disabled with --disable-beauty
The CLI SHALL provide a `--disable-beauty` flag (default `false`). When set, struct-typed cells in table output SHALL render as compact single-line JSON with no ANSI coloring. The flag SHALL have no effect on `--output json` or on scalar cell rendering.

#### Scenario: Flag switches table cells to compact JSON
- **WHEN** `kubectl-sql --disable-beauty "SELECT name, status FROM pods"` is run with table output
- **THEN** the `status` cell contains compact single-line JSON with field names as keys and no indentation or color

#### Scenario: Flag default keeps pretty rendering
- **WHEN** the flag is not passed
- **THEN** struct cells render as pretty-printed JSON

### Requirement: JSON keys are colored in pretty struct cells on TTY
When rendering pretty-printed struct cells in table output, the renderer SHALL colorize JSON object keys (and only keys — never values, braces, or punctuation) with an ANSI color. Coloring SHALL be applied only when stdout is a TTY, `--no-color` is not set, and `--disable-beauty` is not set. CSV and `--output json` output SHALL never contain ANSI escape codes.

#### Scenario: Keys colored on interactive terminal
- **WHEN** `kubectl-sql "SELECT status FROM pods"` runs with stdout attached to a TTY and colors enabled
- **THEN** JSON keys in struct cells are wrapped in ANSI color codes and values are uncolored

#### Scenario: No color when piped
- **WHEN** the same query runs with stdout redirected to a file or pipe
- **THEN** the output contains no ANSI escape codes

#### Scenario: --no-color disables key coloring
- **WHEN** `kubectl-sql --no-color "SELECT status FROM pods"` runs on a TTY
- **THEN** struct cells are pretty-printed JSON with no ANSI escape codes

#### Scenario: Machine formats are never colored
- **WHEN** `kubectl-sql --output csv` or `--output json` is run on a TTY
- **THEN** the output contains no ANSI escape codes
