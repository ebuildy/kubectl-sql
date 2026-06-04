## MODIFIED Requirements

### Requirement: Makefile targets are functional
The `Makefile` SHALL provide targets: `build`, `install`, `lint`, `test`, `test-integration`, `coverage`, `e2e`, `e2e-run-fake`. The `build` target SHALL produce `./bin/kubectl-sql`. The `e2e-run-fake` target SHALL start an envtest API server (via `setup-envtest`), run the `integration`-tagged godog suite, then stop the server.

#### Scenario: Build target produces binary
- **WHEN** `make build` is run
- **THEN** `./bin/kubectl-sql` exists and is executable

#### Scenario: Test target does not run integration tests
- **WHEN** `make test` is run
- **THEN** the command exits 0 and envtest tests are NOT executed

#### Scenario: e2e-run-fake runs integration suite
- **WHEN** `make e2e-run-fake` is run with `setup-envtest` installed
- **THEN** envtest starts, godog scenarios tagged `integration` execute, and the server stops afterward
