## MODIFIED Requirements

### Requirement: SQL query is accepted as a positional argument
The root command SHALL accept a single positional SQL string as its first argument and execute it against the Kubernetes cluster. If the query is `SHOW TABLES` (case-insensitive), it SHALL be handled before the octosql pipeline and return a table of all queryable Kubernetes resource types. Results SHALL be rendered by `internal/output.Render` so the process always exits cleanly regardless of terminal environment. When no positional argument is provided and stdin is a TTY, the command SHALL open the interactive REPL instead of showing help.

#### Scenario: Query executes and prints a table
- **WHEN** the user runs `kubectl-sql "SELECT name, namespace FROM pods"`
- **THEN** the command connects to the cluster, fetches pods, and prints a table with columns `name` and `namespace` to stdout, then exits 0

#### Scenario: Missing argument opens REPL on TTY
- **WHEN** the user runs `kubectl-sql` with no arguments and stdin is a TTY
- **THEN** the command opens the interactive REPL prompt instead of showing help

#### Scenario: Missing argument on non-TTY shows help
- **WHEN** the user runs `kubectl-sql` with no arguments and stdin is not a TTY
- **THEN** the command prints usage help and exits 0

#### Scenario: Invalid SQL prints an error
- **WHEN** the user runs `kubectl-sql "NOT VALID SQL"`
- **THEN** the command prints an error message to stderr and exits 1

#### Scenario: SHOW TABLES is handled before SQL parsing
- **WHEN** the user runs `kubectl-sql "SHOW TABLES"`
- **THEN** the command returns a table of resource types without invoking the octosql pipeline, and exits 0

#### Scenario: Process exits without hanging in any terminal environment
- **WHEN** `kubectl-sql` is run from a VS Code terminal, tmux, SSH session, or any environment without a controlling TTY
- **THEN** the process prints results and exits 0 without hanging
