## Why

The repository has no Go code yet. Before any feature work can begin, the project needs a compilable Go module with the correct structure, toolchain wiring, and a working CLI entrypoint so contributors have a runnable baseline to build on.

## What Changes

- Initialize Go module at `github.com/ebuildy/kubectl-sql`
- Add `main.go` entrypoint wiring cobra
- Add `cmd/root.go` with all flags defined (no-op implementations)
- Scaffold all internal packages as empty stubs (`parser`, `planner`, `executor`, `k8s`, `output`, `debug`) and `pkg/sqlschema`
- Add `Makefile` with `build`, `install`, `lint`, `test`, `test-integration`, `coverage` targets
- Add `docs/grammar.ebnf` skeleton
- Add `test/` directory structure with placeholder files
- Wire `octosql` as the SQL engine dependency (`github.com/cube2222/octosql`)

## Capabilities

### New Capabilities

- `project-scaffold`: Go module, directory layout, Makefile, and cobra CLI entrypoint — `kubectl sql --help` prints usage and exits 0

### Modified Capabilities

_(none — greenfield)_

## Impact

- Creates `go.mod` / `go.sum` with cobra + octosql + k8s client-go dependency tree
- No existing code is touched
- All downstream feature changes depend on this scaffold being in place
