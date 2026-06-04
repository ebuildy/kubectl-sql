## MODIFIED Requirements

### Requirement: CLI entrypoint is reachable
The binary SHALL be named `kubectl-sql` and SHALL print usage information when invoked with `--help`, then exit 0. When a SQL string is provided as the first positional argument, the command SHALL execute the query instead of printing help.

#### Scenario: Help flag works
- **WHEN** `./bin/kubectl-sql --help` is run after `make build`
- **THEN** output contains "kubectl-sql" and usage instructions, and the process exits 0

#### Scenario: No-arg invocation shows help
- **WHEN** `./bin/kubectl-sql` is run with no arguments
- **THEN** output contains usage instructions and the process exits 0

#### Scenario: SQL argument triggers query execution
- **WHEN** `./bin/kubectl-sql "SELECT name FROM pods"` is run
- **THEN** the command executes the query and prints results (not help text), and exits 0 on success
