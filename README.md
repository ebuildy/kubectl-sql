# kubectl-sql

[![Go version](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/ebuildy/kubectl-sql)](https://goreportcard.com/report/github.com/ebuildy/kubectl-sql)
[![CI](https://github.com/ebuildy/kubectl-sql/actions/workflows/ci.yml/badge.svg)](https://github.com/ebuildy/kubectl-sql/actions/workflows/ci.yml)
[![Release](https://github.com/ebuildy/kubectl-sql/actions/workflows/release.yml/badge.svg)](https://github.com/ebuildy/kubectl-sql/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/ebuildy/kubectl-sql?color=6366F1)](https://github.com/ebuildy/kubectl-sql/releases)
[![Downloads](https://img.shields.io/github/downloads/ebuildy/kubectl-sql/total)](https://github.com/ebuildy/kubectl-sql/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/ebuildy/kubectl-sql.svg)](https://pkg.go.dev/github.com/ebuildy/kubectl-sql)

> Query any Kubernetes resource using SQL — directly from your terminal.

```bash
kubectl sql "SELECT name, namespace, status->phase FROM pods WHERE status->phase != 'Running'"
```

`kubectl-sql` is a `kubectl` plugin that brings SQL semantics to Kubernetes. Instead of chaining `kubectl get`, `grep`, `jq`, and `awk`, you write a single declarative query and get back a clean table, JSON, or CSV.

## Features

- **Full SQL subset** — `SELECT`, `WHERE`, `ORDER BY`, `LIMIT`, `GROUP BY`, aggregates (`COUNT`, `SUM`, …), `DISTINCT`
- **Delete or update** - `DELETE` Kubernetes resources with SQL query
- **Dynamic schema** — columns are inferred from the OpenAPI spec (with sample-object fallback), so `SELECT *` returns real resource fields like `status`, `spec`, `metadata`
- **Nested field access** — use `->` for struct traversal (`metadata->labels->app`)
- **Array indexing** — `array_get(spec->volumes, 0)->configMap` resolves the first volume's ConfigMap name
- **All resource types** — built-ins, CRDs, short names, plural forms all accepted
- **Cross-namespace** — queries all namespaces by default; scope with `-n`
- **Multiple output formats** — aligned table, JSON, CSV
- **Introspection** — `SHOW TABLES` and `DESCRIBE TABLE <resource>`

## Installation

### From release binaries

Download the archive for your platform from the [releases page](https://github.com/ebuildy/kubectl-sql/releases) (Linux amd64, macOS amd64/arm64), then:

```bash
tar -xzf kubectl-sql_*.tar.gz
mv kubectl-sql ~/bin/   # or anywhere on your PATH
```

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

Pass a query directly, or run with no query to drop into the interactive REPL:

```
$ kubectl-sql
sql> SELECT name, namespace FROM pods WHERE status->phase != 'Running'
... results ...
sql> /quit
```

At the `sql> ` prompt, type a query and press Enter to run it. Use the up/down
arrows to recall previous queries, and press **Tab** to autocomplete SQL
keywords, table names (after `FROM`), column names of the table in your `FROM`
clause, and slash commands (any word starting with `/`).

REPL slash commands:

| Command | Action |
|---|---|
| `/quit` | exit the REPL (`quit`, `exit`, or `Ctrl-C` also work) |
| `/clear` | clear the screen (history is kept) |
| `/history-clear` | clear the recall history (screen is kept) |
| `/help` | list the slash commands |
| `/version` | print the version and project URL |
| `/tables` | list tables (same as `SHOW TABLES`) |

> **Breaking change:** the old backslash commands `\q`, `\help`, and `?` have
> been removed. Use `/quit` and `/help` instead.

Queries can be piped in too — they run in batch mode, one per line:

```
echo "SELECT name FROM pods LIMIT 5" | kubectl-sql
```

### Web UI

`--ui` starts a small local web server instead of running a query, giving a
no-install graphical way to type queries with syntax highlighting and
autocomplete and read results as an HTML table:

```
kubectl-sql --ui
# kubectl-sql UI listening on http://127.0.0.1:8080
```

Open the printed URL in a browser. The page is fully self-contained (assets are
embedded in the binary — no CDN, no build step). The editor highlights keywords,
strings, and the `->` accessor; Tab requests completions and Ctrl/Cmd+Enter (or
the **Run** button) executes the query.

The server reuses the same cluster configuration as the CLI (`--kubeconfig`,
`--context`, `--namespace`) and runs queries through the same SQL engine in JSON
mode. It is **read-only**: `DELETE` and other mutating statements are rejected
with `403` so the browser cannot trigger destructive operations — use the CLI's
confirmation flow for those. Press Ctrl-C to shut the server down cleanly.

`--ui-address` changes the bind address (default `127.0.0.1:8080`, loopback
only). Binding to a non-loopback address exposes the query API on the network
and prints a warning:

```
kubectl-sql --ui --ui-address 0.0.0.0:9090
```

### Logging

By default only errors are logged. Increase verbosity with `-v` (info) or `-vv`
(debug, including per-step timings in ms). All logs are written to **stderr**, so
query results on stdout stay clean and pipeable:

```
kubectl-sql -vv --output json "SELECT name FROM pods" 2>debug.log | jq .
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `table` | Output format: `table`, `json`, `csv` |
| `--repl` | `-i` | `false` | Open the interactive SQL REPL (default when no query is given) |
| `--ui` | | `false` | Start a local web UI instead of running a query |
| `--ui-address` | | `127.0.0.1:8080` | `host:port` the web UI binds to (loopback by default) |
| `--watch` | `-w` | `false` | Re-run the query every 5s, refreshing the table |
| `--verbose` | `-v` | `error` | Increase log verbosity: `-v`=info, `-vv`=debug. Logs go to stderr |
| `--namespace` | `-n` | all namespaces | Restrict query to a single namespace |
| `--context` | | current context | kubeconfig context to use |
| `--kubeconfig` | | `~/.kube/config` | Path to kubeconfig |
| `--page-size` | | `500` | Kubernetes LIST page size |
| `--timeout` | | `30s` | Per-request timeout |
| `--explain` | | `false` | Print the execution plan without running the query |
| `--dry-run` | | `false` | Validate SQL without hitting the API |
| `--no-color` | | `false` | Disable ANSI colors |
| `--disable-beauty` | | `false` | Render struct values as compact single-line JSON (no pretty-printing or key colors) |

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

Use `->` to traverse struct fields.

```sql
-- Arrow notation
SELECT metadata->labels->app FROM pods

-- Array index access via array_get()
SELECT name, array_get(spec->volumes, 0)->configMap FROM pods WHERE name = 'nginx'
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

### JSON file sources

A `FROM` reference whose table name ends in `.json`, `.jsonl`, or `.ndjson` is read as a
local **JSON Lines** file (one JSON object per line, not a pretty-printed JSON
document). The column schema is inferred by sampling the file, and the result
supports the same `SELECT` column lists, `*`, `WHERE`, `ORDER BY`, `LIMIT`, and
output formats as Kubernetes-backed tables.

```sql
-- Read every line of notes.json as a row
SELECT * FROM notes.json

-- .jsonl and .ndjson are read the same way
SELECT * FROM notes.jsonl
SELECT * FROM notes.ndjson

-- Column selection and filtering work like any other table
SELECT pod, note FROM notes.json WHERE pod = 'nginx-1'

-- JOIN two JSON file tables together
SELECT n.pod, n.note, s.status
FROM notes.json n JOIN status.json s ON n.pod = s.pod
```

Paths may be relative to the current directory (`fixtures/notes.json`), explicitly
relative (`./notes.json`), or absolute (`/tmp/notes.json`).

Each output column is rendered with a `<table>.<field>` prefix, the same convention
used for Kubernetes tables (`pods.name`). For a `.json` file the prefix defaults to
the file's basename (e.g. `notes.pod`); for `.jsonl`/`.ndjson` files the prefix
defaults to the literal extension (`jsonl.pod`/`ndjson.pod`) instead, a quirk of how
`json` is recognized as a SQL keyword but `jsonl`/`ndjson` are not. Use `AS <alias>`
(e.g. `FROM notes.jsonl AS notes`) for a predictable, consistent prefix across all
three extensions.

kubectl-sql registers a Kubernetes database under the name `k8s`, so a file literally
named `k8s.json` (or `k8s.jsonl`/`k8s.ndjson`) would otherwise be interpreted as
resource `json` in the `k8s` database. Reference such a file with a leading `./`:
`FROM ./k8s.json`.

> **Note:** `JOIN` between a Kubernetes-backed table (e.g. `pods`) and a `.json`/
> `.jsonl`/`.ndjson` file is not yet supported — tracked as a follow-up.

## Tips

### Turn `kubectl get -o json` output into JSON Lines

`kubectl get <resource> -o json` returns a single JSON document with the matching
objects nested under `.items`. Pipe it through `jq -c '.items[]'` to flatten that
array into one compact JSON object per line — exactly the JSON Lines format the
[JSON file datasource](#json-file-sources) expects:

```bash
# Snapshot all pods (cluster-wide) to a JSON Lines file
kubectl get pods -A -o json | jq -c '.items[]' > pods.jsonl

# Query the snapshot — no live cluster access needed
kubectl sql "SELECT metadata->name AS name, metadata->namespace AS namespace, status->phase FROM pods.jsonl"
```

This is handy for querying a point-in-time snapshot offline, or for re-running
queries against it without hitting the API server again.

> Note: the `name`/`namespace`/`labels`/`annotations` shortcut columns available on
> `k8s.*` tables (e.g. `pods.name`) are synthesized by kubectl-sql's Kubernetes
> adapter and aren't present in raw `kubectl get -o json` output. Use the underlying
> `metadata->name` / `metadata->namespace` paths (with `AS` to rename) when querying
> a JSON Lines snapshot instead.

### Flatten fields with `jq` before `JOIN`ing snapshots

For `JOIN`s — e.g. diffing two snapshots taken at different times — project the
fields you need into a flat top-level shape with `jq` so the join key (and any
compared columns) are plain fields, not `->` expressions:

```bash
snapshot() {
  kubectl get pods -A -o json |
    jq -c '.items[] | {name: .metadata.name, namespace: .metadata.namespace, phase: .status.phase}'
}

snapshot > before.jsonl
# ... time passes, or changes are rolled out ...
snapshot > after.jsonl

kubectl sql "SELECT b.name, b.namespace, b.phase AS before_phase, a.phase AS after_phase
              FROM before.jsonl b JOIN after.jsonl a ON b.name = a.name
              WHERE b.phase != a.phase"
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

# Delete every Pending pod (previews the set, then asks for confirmation)
kubectl sql "DELETE pod WHERE status->phase = 'Pending'"

# Force-delete with delete options via a MySQL-style hint comment
kubectl sql "DELETE /* force, grace-period=0 */ FROM pod WHERE status->phase = 'Pending'"

# Orphan a deployment's children instead of cascading the delete
kubectl sql "DELETE /* cascade=orphan */ deployment WHERE name = 'web'"

# Skip the confirmation prompt for scripted use (required when non-interactive)
kubectl sql -y "DELETE pod WHERE status->phase = 'Succeeded'"

# Preview the deletion set without deleting anything
kubectl sql --dry-run "DELETE pod WHERE status->phase = 'Pending'"
```

> **Note:** `DELETE` is the only mutating statement and requires the `delete`
> RBAC verb on the target resource. It always previews the matched objects and
> asks for confirmation (default *no*); pass `-y/--yes` to skip the prompt.
> `DELETE` cannot be combined with `--watch`.

## How it works

`kubectl-sql` is built on [octosql](https://github.com/cube2222/octosql), a streaming SQL engine. At query time it:

1. **Infers the schema** from the cluster's OpenAPI v3 spec (primary) or a 1-item LIST sample (fallback), exposing all real resource fields as typed columns
2. **Rewrites the SQL** — bare table names in `FROM`/`JOIN` are qualified with `k8s.` so octosql routes them to the Kubernetes datasource
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

## Specs

Long-lived behavioral specs live in [`openspec/specs/`](openspec/specs/) and are
the source of truth for how each feature works.

| Spec | Description |
|---|---|
| [DELETE Statement](openspec/specs/delete-statement/spec.md) | Defines `DELETE`: grammar with hint-comment options, deletion-set preview, confirmation/`--yes`, bounded-parallel delete, and exit codes. |
| [DESCRIBE TABLE](openspec/specs/describe-table/spec.md) | Lists all columns and types for a resource via `DESCRIBE TABLE <resource>`, inferred from OpenAPI or a sample object. |
| [Dynamic Schema Inference](openspec/specs/dynamic-schema/spec.md) | Defines how resource schemas are inferred at query time, driving column discovery for `SELECT *`, `DESCRIBE TABLE`, and typed filtering. |
| [envtest Integration Tests](openspec/specs/envtest-e2e/spec.md) | Behavioral contract for the envtest-backed integration suite that exercises the full SQL query path without a live cluster. |
| [JSON File Datasource](openspec/specs/json-file-datasource/spec.md) | Defines querying local JSON Lines files (`.json`/`.jsonl`/`.ndjson`) via `FROM <path>`, including `JOIN`s between files. |
| [Kubernetes Datasource](openspec/specs/k8s-datasource/spec.md) | Defines how resource kinds are resolved, fetched, mapped to rows, and namespace-scoped by the Kubernetes datasource layer. |
| [Kubernetes Data-Source Port](openspec/specs/k8s-datasource-port/spec.md) | Defines the hexagonal port/adapter boundary that isolates `client-go`/`apimachinery` and exposes listing, schema, discovery, and a single delete operation. |
| [Logging](openspec/specs/logging/spec.md) | Defines leveled `-v`/`-vv` logging to stderr, shared via context, behind a port/adapter boundary, with timed debug/info traces. |
| [Output Renderer](openspec/specs/output-renderer/spec.md) | Defines `internal/output.Render`, the TTY-independent renderer that drives execution and writes query results. |
| [Project Scaffold](openspec/specs/project-scaffold/spec.md) | Baseline structural requirements: Go module setup, CLI entrypoint, flags, package layout, and Makefile targets. |
| [Query Typo Suggestion](openspec/specs/query-typo-suggestion/spec.md) | Turns a failed query into a high-confidence single-token correction (keyword, table, field, dotted access, or unterminated quote) by string similarity. |
| [SHOW TABLES](openspec/specs/show-tables/spec.md) | Defines `SHOW TABLES`, which lists all Kubernetes API resource types queryable via `kubectl-sql`. |
| [SQL Engine Port](openspec/specs/sql-engine-port/spec.md) | Defines the hexagonal port/adapter boundary that confines the octosql engine and keeps it swappable. |
| [SQL Execution](openspec/specs/sql-execution/spec.md) | End-to-end SQL query execution contract: CLI input, SELECT/WHERE/LIMIT semantics, DELETE routing to the mutator adapter, and flag forwarding. |
| [SQL Mutator Adapter](openspec/specs/sql-mutator-adapter/spec.md) | Defines the `mutator` adapter that owns mutating statements, resolving targets via octosql and deleting through the DataSource port with bounded parallelism. |
| [SQL REPL](openspec/specs/sql-repl/spec.md) | Defines the interactive REPL: prompt loop, slash commands (`/quit`, `/clear`, `/history-clear`, `/help`, `/version`, `/tables`), history, batch fallback, and Tab autocomplete. |
| [Swagger Schema Provider](openspec/specs/swagger-schema-provider/spec.md) | Embeds a generated Kubernetes OpenAPI snapshot so `spec`/`status` field structure is available without a cluster round trip. |
| [Watch Mode](openspec/specs/watch-mode/spec.md) | Defines the `--watch`/`-w` flag, which re-executes the query every 5 seconds and reprints the result table until Ctrl-C or `--timeout`. |
| [Web UI API](openspec/specs/web-ui-api/spec.md) | Defines the JSON API behind the web UI: `POST /api/query` (run SQL, return columns/rows or structured errors) and `GET /api/complete`, with mutating statements rejected. |
| [Web UI Command](openspec/specs/web-ui-command/spec.md) | Defines the `--ui`/`--ui-address` flags and the local web server lifecycle: startup banner, browser launch, query pre-load, config reuse, and graceful shutdown. |
| [Web UI Page](openspec/specs/web-ui-page/spec.md) | Defines the embedded single-page UI: a framework-free, syntax-highlighted SQL editor with autocomplete and an HTML results table with colored-YAML cells and resizable columns. |

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

### Regenerating the embedded Kubernetes schema

`kubectl-sql` ships with an embedded snapshot of the Kubernetes OpenAPI v2 spec
(`internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go` +
`.bin.gz`), used as a schema source for `DESCRIBE TABLE` and `SELECT *` column
inference on built-in resources.

To regenerate it:

```bash
make generate
```

This runs [`tools/genk8sschema`](tools/genk8sschema), which reads
`internal/adapter/datasources/k8s/testdata/swagger.json` and writes the
embedded Go snapshot. That fixture is gitignored; if it's missing, `make
generate` downloads the latest `swagger.json` from
[kubernetes/kubernetes master](https://github.com/kubernetes/kubernetes/blob/master/api/openapi-spec/swagger.json)
automatically.

To refresh the snapshot to a newer Kubernetes version, delete the fixture and
re-run `make generate`:

```bash
rm internal/adapter/datasources/k8s/testdata/swagger.json
make generate
```

## Releasing

Releases are fully automated with [GoReleaser](https://goreleaser.com/) via the [Release workflow](.github/workflows/release.yml). Pushing a tag matching `v*` triggers it:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The workflow then:

1. Builds static binaries (`CGO_ENABLED=0`) for **linux/amd64**, **darwin/amd64**, and **darwin/arm64**
2. Packages each as a `tar.gz` archive with the `LICENSE` and `README.md`
3. Generates a `checksums.txt` (SHA-256) for all archives
4. Creates a GitHub release with a changelog from commit messages (`docs:`, `test:`, and `chore:` commits excluded)

Tags with a prerelease suffix (e.g. `v0.2.0-rc1`) are automatically marked as prereleases. Build targets and packaging are configured in [.goreleaser.yaml](.goreleaser.yaml); validate changes locally with `goreleaser check` or do a full dry run with `goreleaser release --snapshot --clean`.

## Documentation

| Document | Description |
|---|---|
| [ADR-001 — Schema inference strategy](docs/adr-001-schema-inference-strategy.md) | Why OpenAPI is the primary schema source with sample-object fallback |
| [ADR-002 — SQL engine choice](docs/adr-002-octosql-sql-engine.md) | Why octosql, and why DuckDB was considered but ruled out |
| [ADR-003 — Go over Rust](docs/adr-003-go-versus-rust.md) | Language choice rationale: velocity, Kubernetes ecosystem, static binary |
| [ADR-004 — AI-assisted development](docs/adr-004-ai-assisted-development.md) | How Claude Code + OpenSpec were used to build this project |
| [SQL grammar (EBNF)](docs/grammar.ebnf) | Formal grammar reference |
| [OpenSpec behavioral specs](openspec/specs/) | Long-lived specs for all features |

---

⚡ Made blazing fast with love at Sanary-sur-Mer 🌊
