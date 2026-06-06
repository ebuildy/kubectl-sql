# kubectl-sql

[![Go version](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> Query any Kubernetes resource using SQL — directly from your terminal.

```bash
kubectl sql "SELECT name, namespace, status->phase FROM pods WHERE status->phase != 'Running'"
```

`kubectl-sql` is a `kubectl` plugin that brings SQL semantics to Kubernetes. Instead of chaining `kubectl get`, `grep`, `jq`, and `awk`, you write a single declarative query and get back a clean table, JSON, or CSV.

## Features

- **Full SQL subset** — `SELECT`, `WHERE`, `ORDER BY`, `LIMIT`, `GROUP BY`, aggregates (`COUNT`, `SUM`, …), `DISTINCT`
- **Dynamic schema** — columns are inferred from the OpenAPI spec (with sample-object fallback), so `SELECT *` returns real resource fields like `status`, `spec`, `metadata`
- **Nested field access** — use `->` for struct traversal (`metadata->labels->app`) or dot notation (auto-rewritten)
- **Array indexing** — `spec.volumes[0].configMap` resolves the first volume's ConfigMap name
- **All resource types** — built-ins, CRDs, short names, plural forms all accepted
- **Cross-namespace** — queries all namespaces by default; scope with `-n`
- **Multiple output formats** — aligned table, JSON, CSV
- **Introspection** — `SHOW TABLES` and `DESCRIBE TABLE <resource>`

## Installation

### From source

```bash
git clone https://github.com/ebuildy/kubectl-sql
cd kubectl-sql
make build    # produces ./bin/kubectl-sql
make install  # copies to ~/bin — ensure ~/bin is on your PATH
```

### As a kubectl plugin

Once `kubectl-sql` is on your `PATH`, kubectl picks it up automatically:

```bash
kubectl sql "SELECT name FROM pods"
```

## Usage

```
kubectl-sql [query] [flags]
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `table` | Output format: `table`, `json`, `csv` |
| `--namespace` | `-n` | all namespaces | Restrict query to a single namespace |
| `--context` | | current context | kubeconfig context to use |
| `--kubeconfig` | | `~/.kube/config` | Path to kubeconfig |
| `--page-size` | | `500` | Kubernetes LIST page size |
| `--timeout` | | `30s` | Per-request timeout |
| `--explain` | | `false` | Print the execution plan without running the query |
| `--dry-run` | | `false` | Validate SQL without hitting the API |
| `--no-color` | | `false` | Disable ANSI colors |

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Query or parse error |
| `2` | Kubernetes API error |

## SQL Reference

### Basic queries

```sql
-- List all pods across all namespaces
SELECT name, namespace FROM pods

-- Filter by field value
SELECT name, namespace, status->phase FROM pods WHERE status->phase = 'Running'

-- Sort and limit
SELECT name, namespace FROM pods ORDER BY name LIMIT 20

-- Wildcard — returns all inferred columns
SELECT * FROM deployments LIMIT 5
```

### Nested fields

Use `->` to traverse struct fields. Dot notation is automatically rewritten.

```sql
-- Arrow notation
SELECT metadata->labels->app FROM pods

-- Dot notation (equivalent)
SELECT metadata.labels.app FROM pods

-- Array index access
SELECT name, spec.volumes[0].configMap FROM pods WHERE name = 'nginx'
```

### Aggregates

```sql
-- Count pods per namespace
SELECT namespace, COUNT(*) FROM pods GROUP BY namespace

-- Total replicas across all deployments
SELECT SUM(status->replicas) FROM deployments
```

### Label and annotation selectors

```sql
SELECT * FROM pods WHERE LABEL 'app' = 'nginx'
SELECT * FROM pods WHERE ANNOTATION 'team' = 'platform'
```

### Introspection

```sql
-- List all queryable resource types
SHOW TABLES

-- List columns and types for a resource
DESCRIBE TABLE pods
DESCRIBE TABLE deployments
```

## Recipes

```bash
# Pods not Running
kubectl sql "SELECT name, namespace, status->phase FROM pods WHERE status->phase != 'Running'"

# Recent warning events
kubectl sql "SELECT name, namespace, reason, message FROM events WHERE type = 'Warning' ORDER BY lastTimestamp DESC LIMIT 50"

# CrashLoopBackOff containers
kubectl sql "SELECT name, namespace FROM pods WHERE status->containerStatuses->0->state->waiting->reason = 'CrashLoopBackOff'"

# Deployments with unavailable replicas
kubectl sql "SELECT name, namespace, status->replicas, status->availableReplicas FROM deployments WHERE status->availableReplicas < status->replicas"

# Pods in a specific namespace
kubectl sql -n kube-system "SELECT name, status->phase FROM pods"

# Count pods per namespace
kubectl sql "SELECT namespace, COUNT(*) FROM pods GROUP BY namespace"

# JSON output for scripting
kubectl sql -o json "SELECT name, namespace FROM pods WHERE status->phase = 'Failed'"

# Dry-run to validate SQL before hitting the cluster
kubectl sql --dry-run "SELECT name FROM doesnotexist"

# Show execution plan
kubectl sql --explain "SELECT name FROM pods WHERE status->phase = 'Pending'"
```

## How it works

`kubectl-sql` is built on [octosql](https://github.com/cube2222/octosql), a streaming SQL engine. At query time it:

1. **Infers the schema** from the cluster's OpenAPI v3 spec (primary) or a 1-item LIST sample (fallback), exposing all real resource fields as typed columns
2. **Rewrites the SQL** — dot-notation field paths become octosql `->` struct access operators; array index paths become flat column names
3. **Streams results** — resources are fetched with paginated LIST calls and streamed through the SQL engine; no full cluster load into memory
4. **Renders output** — results are written as an aligned table, JSON array, or CSV

Schema inference uses a hexagonal architecture: `OpenAPIInferrer` → `SampleInferrer` → `CompositeInferrer`, so any resource type — including CRDs without a formal schema — works out of the box.

## Built with Claude Code + OpenSpec

This project was built entirely with [Claude Code](https://claude.ai/claude-code) using a spec-driven workflow called [OpenSpec](https://openspec.pro/).

Every non-trivial feature followed this cycle:

1. **Propose** — describe the change in plain language; Claude generates `proposal.md`, `design.md`, and behavioral specs (`specs/*.md`)
2. **Apply** — Claude implements the tasks in `tasks.md` one by one, guided by the specs
3. **Archive** — completed changes are archived and their specs are merged into the long-lived `openspec/specs/` source of truth

The specs live in [`openspec/`](openspec/) alongside the code. They document *what* the system does and *why* decisions were made — independently of any AI session. See [`docs/adr-001-schema-inference-strategy.md`](docs/adr-001-schema-inference-strategy.md) for an example of an Architecture Decision Record produced during this process.

> [!NOTE]
> The entire codebase — from project scaffold to schema inference to the SQL rewriter — was produced through conversational iteration with Claude Code, with humans reviewing and steering at each step.

## Development

```bash
# Run unit tests
make test

# Run integration tests (requires envtest)
make test-integration

# Run end-to-end tests against a local envtest cluster
make e2e-run-fake

# Lint
make lint

# Install dev dependencies (golangci-lint, setup-envtest)
make dev-deps
```

> [!NOTE]
> Integration and e2e tests use [controller-runtime envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest) — no real cluster needed. Run `make dev-deps` first to download the required binaries.
