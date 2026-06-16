## ADDED Requirements

### Requirement: One-shot queries offer a typo correction
The command SHALL, when a one-shot CLI query (a positional SQL argument routed through the octosql pipeline) fails because of a single mistyped keyword, table name, or field and a close valid match is found, emit a correction suggestion governed by the `query-typo-suggestion` capability rather than printing only the raw error. On an interactive terminal the command SHALL prompt to run the corrected query (default yes) and, on confirmation, execute it and render its results. When the session is non-interactive (piped stdin / batch) the command SHALL print the suggestion line and exit 1 without running the corrected query. When no close match exists the command SHALL print the original error and exit 1 as before.

#### Scenario: Interactive one-shot query suggests and runs the fix on confirmation
- **WHEN** the user runs `kubectl-sql "SELECT staus FROM pods"` on a TTY and `status` is the closest valid field
- **THEN** the command prints `error: field staus does not exist, run this query instead ? SELECT status FROM pods` and, on confirmation, executes `SELECT status FROM pods` and prints the results

#### Scenario: Table-name typo is suggested on the one-shot path
- **WHEN** the user runs `kubectl-sql "SELECT name FROM pdos"` on a TTY and `pods` is the closest queryable resource
- **THEN** the command suggests `SELECT name FROM pods` and runs it on confirmation

#### Scenario: Non-interactive one-shot query prints the suggestion but does not run it
- **WHEN** the user runs `kubectl-sql "SELECT staus FROM pods"` with piped stdin
- **THEN** the command prints the suggestion line and exits 1 without executing the corrected query

#### Scenario: Typo with no close match keeps the original error
- **WHEN** a one-shot query has a mistyped token with no valid match within the similarity threshold
- **THEN** the command prints the original error and exits 1
