# Spec: Project Scaffold

## Purpose

Defines the baseline structural requirements for the `kubectl-sql` project: Go module setup, CLI entrypoint, flag definitions, package layout, and Makefile targets. These requirements must be satisfied by the initial scaffold and remain true throughout the project lifecycle.

---

## Requirements

### Requirement: Go module is initialised
The project SHALL have a valid `go.mod` file declaring module `github.com/ebuildy/kubectl-sql` with Go 1.23 or later, and SHALL include `github.com/cube2222/octosql`, `github.com/spf13/cobra`, and `k8s.io/client-go` as direct dependencies.

#### Scenario: Module compiles cleanly
- **WHEN** `go build ./...` is run from the repository root
- **THEN** the command exits 0 with no errors

#### Scenario: Dependencies are present
- **WHEN** `go list -m all` is run
- **THEN** output includes `github.com/cube2222/octosql`, `github.com/spf13/cobra`, and `k8s.io/client-go`

---

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

---

### Requirement: All CLI flags are defined
The root command SHALL define all flags specified in AGENTS.md: `--output/-o`, `--context`, `--namespace/-n`, `--kubeconfig`, `--page-size`, `--timeout`, `--no-color`, `--explain`, `--dry-run`.

#### Scenario: Flags appear in help output
- **WHEN** `./bin/kubectl-sql --help` is run
- **THEN** all nine flags are listed in the output with their default values

---

### Requirement: Package layout matches project structure
All packages defined in AGENTS.md SHALL exist and be importable: `internal/parser`, `internal/planner`, `internal/executor`, `internal/k8s`, `internal/output`, `internal/debug`, `pkg/sqlschema`.

#### Scenario: All packages compile
- **WHEN** `go build ./...` is run
- **THEN** all packages compile without errors

---

### Requirement: Makefile targets are functional
The `Makefile` SHALL provide targets: `build`, `install`, `lint`, `test`, `test-integration`, `coverage`. The `build` target SHALL produce `./bin/kubectl-sql`.

#### Scenario: Build target produces binary
- **WHEN** `make build` is run
- **THEN** `./bin/kubectl-sql` exists and is executable

#### Scenario: Test target runs without error on empty suite
- **WHEN** `make test` is run on the initial scaffold
- **THEN** the command exits 0 (no test failures; no tests yet is acceptable)
