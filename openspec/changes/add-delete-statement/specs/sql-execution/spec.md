## MODIFIED Requirements

### Requirement: SQL query is accepted as a positional argument
The root command SHALL accept a single positional SQL string as its first argument and execute it against the Kubernetes cluster. If the query is `SHOW TABLES` (case-insensitive), it SHALL be handled before the octosql pipeline and return a table of all queryable Kubernetes resource types. If the query is a `DESCRIBE TABLE <resource>` statement, it SHALL likewise be handled before the octosql pipeline. If the query is a `DELETE [FROM] <resource> [WHERE <expr>]` statement (case-insensitive), it SHALL also be intercepted before the octosql pipeline and routed to the `mutator` SQL adapter, which runs the deletion flow (preview, confirmation, delete) governed by the `delete-statement` and `sql-mutator-adapter` specs; octosql SHALL NOT be asked to parse it, since octosql is SELECT-only. Results SHALL be rendered by `internal/output.Render` so the process always exits cleanly regardless of terminal environment. When no positional argument is provided, the command SHALL open the REPL: interactively on a TTY, or in line-by-line batch mode when stdin is piped.

#### Scenario: Query executes and prints a table
- **WHEN** the user runs `kubectl-sql "SELECT name, namespace FROM pods"`
- **THEN** the command connects to the cluster, fetches pods, and prints a table with columns `name` and `namespace` to stdout, then exits 0

#### Scenario: Missing argument opens REPL on TTY
- **WHEN** the user runs `kubectl-sql` with no arguments and stdin is a TTY
- **THEN** the command opens the interactive REPL prompt instead of showing help

#### Scenario: Missing argument with piped stdin runs batch mode
- **WHEN** the user runs `kubectl-sql` with no arguments and stdin is piped (not a TTY)
- **THEN** the command reads queries line-by-line from stdin, executes each, and exits 0 at EOF

#### Scenario: Invalid SQL prints an error
- **WHEN** the user runs `kubectl-sql "NOT VALID SQL"`
- **THEN** the command prints an error message to stderr and exits 1

#### Scenario: SHOW TABLES is handled before SQL parsing
- **WHEN** the user runs `kubectl-sql "SHOW TABLES"`
- **THEN** the command returns a table of resource types without invoking the octosql pipeline, and exits 0

#### Scenario: DELETE is routed to the mutator adapter
- **WHEN** the user runs `kubectl-sql "DELETE pod WHERE status->phase = 'Pending'"`
- **THEN** the command dispatches the statement to the `mutator` SQL adapter rather than parsing it as a SELECT through octosql

#### Scenario: Process exits without hanging in any terminal environment
- **WHEN** `kubectl-sql` is run from a VS Code terminal, tmux, SSH session, or any environment without a controlling TTY
- **THEN** the process prints results and exits 0 without hanging
