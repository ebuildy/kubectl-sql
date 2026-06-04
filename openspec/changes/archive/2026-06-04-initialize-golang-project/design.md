## Context

Greenfield Go project. The repository currently contains only `AGENTS.md`, `CLAUDE.md`, and an empty OpenSpec scaffold. This design covers the one-time bootstrap of the Go module and project skeleton that all future feature changes will build on.

## Goals / Non-Goals

**Goals:**
- Produce a compilable Go module at `github.com/ebuildy/kubectl-sql`
- Wire cobra so `kubectl sql --help` works end-to-end
- Define all CLI flags as stubs (no logic behind them yet)
- Establish the package layout from AGENTS.md so future changes have a home
- Pin `octosql` as the SQL engine and `client-go` for k8s access
- Provide a Makefile that makes `build`, `test`, `lint` runnable from day one

**Non-Goals:**
- Any actual SQL parsing, planning, or execution logic
- Real Kubernetes API calls
- Output formatting beyond cobra's default help text
- Integration test harness (envtest setup is a later change)

## Decisions

### SQL engine: octosql (`github.com/cube2222/octosql`)

Chosen by the project owner over a hand-written parser. OctoSQL provides a full SQL execution engine that accepts pluggable data sources — the k8s dynamic client will be wired as a datasource in a future change. This avoids writing and maintaining a hand-rolled SQL parser and planner.

**Alternative considered**: hand-written lexer/parser/planner (as originally described in AGENTS.md). Rejected in favor of octosql to reduce implementation surface.

**Impact on AGENTS.md layout**: the `internal/parser/`, `internal/planner/` packages will exist as stubs but may eventually be reduced or removed once octosql is fully wired. The `internal/executor/` package will become the octosql datasource adapter.

### CLI framework: cobra

Standard choice for kubectl plugins. Supports persistent flags, subcommands, and the `--help` contract required by the kubectl plugin UX spec.

### k8s client: `k8s.io/client-go` dynamic client

Unchanged from AGENTS.md conventions. The dynamic client handles CRDs automatically without code generation.

### Stub depth: hello-world only

All internal packages will contain a single `.go` file with the package declaration and a `// TODO` comment. No interfaces, no types, no logic. This keeps the initial PR reviewable and avoids locking in API shapes before the design is done.

## Risks / Trade-offs

- **octosql API compatibility** → octosql is under active development; its datasource interface may change. Mitigation: pin to a specific version in `go.mod` and track upstream releases.
- **Heavy dependency tree** → pulling in octosql + client-go produces a large `go.sum`. This is expected and not a problem for a CLI tool.
- **AGENTS.md describes a hand-written parser** → the spec files in `openspec/specs/` will need to be updated to reflect octosql ownership of the SQL layer. This is deferred to the first SQL feature change.

## Open Questions

- Which version of octosql to pin? (resolve at `go get` time — use latest stable)
- Should `go.sum` be committed? Yes — standard practice for CLI tools.
