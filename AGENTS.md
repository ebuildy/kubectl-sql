# AGENTS.md — kubectl-sql

> AI assistant instructions for the `kubectl-sql` project.  
> Maintained alongside code. Updated when conventions change.  
> Spec framework: [OpenSpec](https://openspec.pro/) — spec-driven development for AI coding assistants.

---

## Project Overview

`kubectl-sql` is a `kubectl` plugin written in Go that lets users query any Kubernetes resource
using SQL-like syntax. It is designed for fast debugging, error discovery, resource listing, and
cross-namespace analysis directly from the terminal.

```
kubectl sql "SELECT name, namespace, status.phase FROM pods WHERE status.phase != 'Running'"
kubectl sql "SELECT name, age FROM nodes WHERE .metadata.labels['node-role.kubernetes.io/master'] IS NOT NULL"
kubectl sql "SELECT name, namespace, reason, message FROM events WHERE type = 'Warning' ORDER BY lastTimestamp DESC LIMIT 20"
```

---

## Repository Layout

```
kubectl-sql/
├── AGENTS.md                    ← you are here
├── README.md
├── go.mod / go.sum
├── main.go                      ← CLI entrypoint (cobra)
│
├── cmd/
│   └── root.go                  ← root cobra command, flags, help
│
├── internal/
│   ├── parser/                  ← SQL → AST (SELECT, FROM, WHERE, ORDER, LIMIT)
│   │   ├── lexer.go
│   │   ├── parser.go
│   │   └── ast.go
│   ├── planner/                 ← AST → execution plan (resource kind, filters)
│   │   └── planner.go
│   ├── executor/                ← plan → k8s API calls → result rows
│   │   ├── executor.go
│   │   └── resolver.go          ← JSON path field resolution on unstructured objects
│   ├── k8s/                     ← Kubernetes client bootstrap (kubeconfig, contexts)
│   │   └── client.go
│   ├── output/                  ← result rendering: table, json, yaml, csv
│   │   └── renderer.go
│   └── debug/                   ← error enrichment, log helpers
│       └── enricher.go
│
├── pkg/
│   └── sqlschema/               ← public: well-known field aliases, type hints
│       └── schema.go
│
├── openspec/
│   ├── specs/                   ← long-lived behavioral specs (keep up to date)
│   │   ├── sql-grammar.md
│   │   ├── resource-resolution.md
│   │   ├── output-formats.md
│   │   └── error-enrichment.md
│   └── changes/                 ← active and archived feature changes
│       └── archive/
│
├── test/
│   ├── unit/
│   ├── integration/             ← uses envtest (controller-runtime)
│   └── fixtures/                ← sample kubeconfigs, YAML snapshots
│
└── docs/
    └── grammar.ebnf             ← formal SQL subset grammar
```

---

## OpenSpec Workflow

This project uses **OpenSpec** for spec-driven development. Every non-trivial feature starts as a
change, not as a code edit.

### Quick reference

| Command | What it does |
|---|---|
| `/opsx:new <slug>` | Create a new change folder under `openspec/changes/<slug>/` |
| `/opsx:ff` | Fast-forward: generate `proposal.md`, `specs/`, `design.md`, `tasks.md` in one pass |
| `/opsx:apply` | Implement all tasks in `tasks.md` following the specs and design |
| `/opsx:archive` | Move completed change to `openspec/changes/archive/` and update long-lived specs |

### When to create a change

- Adding or modifying SQL grammar (new clause, function, operator)
- New output format or renderer
- New Kubernetes resource type or API group support
- Changes to error enrichment logic
- Refactoring that touches more than two packages
- Any new CLI flag

### Change folder structure

```
openspec/changes/<slug>/
├── proposal.md     ← problem, scope, risks (readable in 2 minutes)
├── specs/          ← Given/When/Then scenarios — the behavioral contract
│   └── *.md
├── design.md       ← components, data flow, tradeoffs
└── tasks.md        ← ordered implementation checklist for /opsx:apply
```

### Long-lived specs (`openspec/specs/`)

These are the source of truth for stable behavior. After archiving a change, reconcile any
behavioral deltas back into the relevant spec file here.

| Spec file | Covers |
|---|---|
| `sql-grammar.md` | Supported SQL syntax, operators, functions, reserved words |
| `resource-resolution.md` | How FROM maps to k8s API groups/versions, CRD discovery |
| `output-formats.md` | table / json / yaml / csv behavior, column truncation rules |
| `error-enrichment.md` | How raw k8s errors are enriched with context and suggestions |

---

## Coding Conventions

### Language and toolchain

- **Go 1.23+** — use the version pinned in `go.mod`
- **`golangci-lint`** — run before every commit: `make lint`
- **No global state** — all dependencies injected via constructors or context
- **Errors wrapped with context** — `fmt.Errorf("planner: %w", err)` at every boundary

### Naming

- Packages: short, lowercase, no underscores (`parser`, `executor`, `k8s`)
- Exported types: full descriptive names (`SQLQuery`, `ExecutionPlan`, `RowSet`)
- Internal helpers: unexported, verb-first (`resolveField`, `buildFilter`)
- Test files: `<file>_test.go` in the same package (white-box) or `_test` package (black-box)

### SQL parser

- Grammar is defined in `docs/grammar.ebnf` — update it before changing the parser
- The lexer and parser are hand-written (no yacc/antlr) for minimal dependency footprint
- AST nodes live in `internal/parser/ast.go` and must be serialisable (implement `fmt.Stringer`)
- Parser errors must include line + column position

### Kubernetes client

- Use `k8s.io/client-go` dynamic client for all resource access (supports CRDs automatically)
- Never hardcode API group versions — discover them via the REST mapper
- Always respect `--context`, `--namespace`, and `--kubeconfig` flags
- Paginate LIST calls (default page size: 500) — never load the entire cluster into memory at once

### Field resolution (`internal/executor/resolver.go`)

- Fields are JSON paths on `unstructured.Unstructured` objects
- Support dot notation: `status.phase`, `.metadata.labels['app']`
- Unknown fields return `NULL` (not an error) so WHERE filters work gracefully
- Type coercion: numbers, booleans, RFC3339 timestamps, and strings

### Output

- Default format: aligned table (auto-detected terminal width)
- Machine-readable: `--output json|yaml|csv`
- Never truncate JSON/YAML output; truncate table cells at 64 chars with `…`
- Colors only when stdout is a TTY (`--no-color` flag always respected)

### Error enrichment (`internal/debug/enricher.go`)

- Every k8s API error gets annotated with: resource kind, namespace, likely cause, suggestion
- `ImagePullBackOff` → suggest `kubectl describe pod` + registry auth check
- `CrashLoopBackOff` → suggest log command + exit code meaning
- `OOMKilled` → suggest memory limit increase

---

## Build and Test

```bash
# Build
make build                  # produces ./bin/kubectl-sql

# Install locally as kubectl plugin
make install                # copies to ~/bin (must be on PATH)

# Lint
make lint                   # golangci-lint run ./...

# Unit tests
make test                   # go test ./... -race -count=1

# Integration tests (requires a running cluster or envtest)
make test-integration       # uses KUBECONFIG from environment

# Coverage
make coverage               # opens HTML coverage report
```

### Testing rules

- Every new parser feature: unit tests in `test/unit/parser/`
- Every new SQL operator or function: at least one positive and one negative test
- Executor tests use `envtest` with fixture objects — no real cluster required
- No `t.Skip()` without a linked issue comment
- Test helper factories live in `test/fixtures/` — reuse them, never inline raw YAML in tests

---

## CLI Design

The plugin must conform to the `kubectl` plugin UX contract:

- Binary name: `kubectl-sql` (hyphen, not underscore)
- Installed on PATH → invoked as `kubectl sql`
- Respects all standard kubectl flags: `--kubeconfig`, `--context`, `--namespace`, `--token`
- Exit codes: `0` success, `1` query/parse error, `2` k8s API error
- `--help` on every subcommand

### Flags

| Flag | Default | Description |
|---|---|---|
| `--output / -o` | `table` | Output format: `table`, `json`, `yaml`, `csv` |
| `--context` | current context | kubeconfig context to use |
| `--namespace / -n` | `""` (all) | Restrict query to a single namespace |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig |
| `--page-size` | `500` | k8s LIST page size |
| `--timeout` | `30s` | Per-request timeout |
| `--no-color` | false | Disable ANSI colors |
| `--explain` | false | Print execution plan without running query |
| `--dry-run` | false | Validate SQL without hitting the API |

---

## SQL Grammar Reference (summary)

Full EBNF lives in `docs/grammar.ebnf`. Key rules for the assistant:

```sql
-- Basic listing
SELECT <fields> FROM <resource_kind>

-- Filtering
SELECT ... FROM ... WHERE <expr>

-- Sorting and limiting
SELECT ... FROM ... ORDER BY <field> [ASC|DESC] LIMIT <n>

-- Cross-namespace (default when -n is not passed)
SELECT ... FROM pods                   -- all namespaces
SELECT ... FROM pods IN NAMESPACE "kube-system"

-- Resource shortnames and plural forms accepted
FROM pod / pods / po

-- Field wildcards
SELECT * FROM deployments

-- Aggregates (v1 scope)
SELECT COUNT(*) FROM pods WHERE status.phase = 'Failed'

-- Label selector sugar
SELECT * FROM pods WHERE LABEL 'app' = 'nginx'

-- Annotation selector
SELECT * FROM pods WHERE ANNOTATION 'team' = 'platform'
```

---

## Guardrails for the AI Assistant

1. **Read specs before coding.** If a relevant `openspec/specs/*.md` or change `specs/` file
   exists, read it first. Do not infer behavior from existing code alone.

2. **Do not modify `openspec/specs/` during a change.** Long-lived specs are updated only during
   `/opsx:archive` to reflect what actually shipped.

3. **Do not add dependencies without noting them in `design.md`.** Every new `go.mod` dependency
   must be justified in the active change's design doc.

4. **Do not write generated code by hand.** If a file has a `// Code generated` header, regenerate
   it via the appropriate `make generate` target instead.

5. **Preserve backward compatibility.** The SQL grammar is a public interface — removing or
   renaming clauses is a breaking change and requires a proposal.

6. **Security.** The plugin only performs read operations (LIST, GET, WATCH). It must never
   write, patch, delete, or exec into any resource. If asked to add a write path, create a
   change with a proposal first and flag it explicitly.

7. **Context resets between planning and coding.** After `/opsx:ff`, start a fresh session
   referencing the spec files — do not implement directly in the planning thread.

8. **One change = one responsibility.** Do not bundle grammar changes with output format changes.
   Keep changes narrow and reviewable.

---

## Common Debug Recipes (for README generation and docs)

```bash
# List all failing pods across the cluster
kubectl sql "SELECT name, namespace, status.phase, status.reason FROM pods WHERE status.phase = 'Failed'"

# Find recent warnings
kubectl sql "SELECT name, namespace, reason, message, lastTimestamp FROM events WHERE type = 'Warning' ORDER BY lastTimestamp DESC LIMIT 50"

# Nodes not Ready
kubectl sql "SELECT name, status.conditions[?type=='Ready'].status AS ready, .metadata.labels['kubernetes.io/arch'] AS arch FROM nodes WHERE ready != 'True'"

# CrashLoopBackOff containers
kubectl sql "SELECT name, namespace, .status.containerStatuses[0].state.waiting.reason AS reason FROM pods WHERE reason = 'CrashLoopBackOff'"

# Deployments with unavailable replicas
kubectl sql "SELECT name, namespace, status.replicas, status.availableReplicas FROM deployments WHERE status.availableReplicas < status.replicas"

# Show execution plan (no API calls)
kubectl sql --explain "SELECT name FROM pods WHERE status.phase = 'Pending'"
```

---

## Contributing Flow (with OpenSpec)

```
1. /opsx:new <slug>          # e.g. /opsx:new add-aggregate-functions
2. /opsx:ff                  # generate proposal + specs + design + tasks
3. Review output carefully    # adjust scope, add/remove scenarios
4. Start fresh session        # attach openspec/changes/<slug>/specs/ files
5. /opsx:apply               # implement tasks one by one
6. make lint && make test     # must pass clean
7. Open PR — include link to change folder in PR description
8. /opsx:archive             # after merge, reconcile openspec/specs/
```

---

*Last updated: 2026-06-04 — reconcile after each `/opsx:archive`.*