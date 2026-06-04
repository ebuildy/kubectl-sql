## Context

The scaffold has cobra, octosql, and client-go wired as dependencies but none of the packages contain real logic. This design connects them: cobra receives the SQL string → octosql parses and executes it → a custom `DataSource` fetches rows from Kubernetes via client-go → octosql formats and prints the table.

## Goals / Non-Goals

**Goals:**
- `kubectl sql "SELECT ..."` executes the query and prints a table to stdout
- The FROM clause resource name is resolved to a Kubernetes API group/version via REST mapper
- All kubeconfig flags (`--context`, `--namespace`, `--kubeconfig`) are respected
- Pagination via `--page-size` flag (default 500)
- Per-request timeout via `--timeout` flag (default 30s)

**Non-Goals:**
- JOINs across multiple resource types (octosql supports this but k8s multi-resource is a later change)
- `--output json|yaml|csv` format switching (table only for this change)
- `--explain` / `--dry-run` flag implementation
- Error enrichment (`internal/debug/`) — raw errors are acceptable for now
- CRD discovery beyond what REST mapper provides

## Decisions

### octosql datasource interface

octosql v0.13 exposes a `datasources.DataSourceRepository` that maps table names to `physical.DataSource` implementations. Each datasource implements `Get(ctx, schema, variables) (physical.Node, error)`.

We implement `KubernetesDataSource` in `internal/executor/` that:
1. Takes a resource kind string (from the FROM clause table name)
2. Uses the REST mapper to resolve it to a GVR (GroupVersionResource)
3. Calls `dynamicClient.Resource(gvr).Namespace(ns).List(ctx, opts)` with pagination
4. Returns each item's unstructured JSON as an octosql `Object` row

**Alternative considered**: use octosql's CSV/JSON file datasource as a shim (pipe `kubectl get -o json` output). Rejected — subprocess shelling is fragile, doesn't respect context/namespace flags cleanly, and bypasses pagination.

### Field mapping strategy

Each Kubernetes object's full `.Object` map (the raw `map[string]interface{}` from `unstructured.Unstructured`) is passed directly as an octosql `Object`. octosql's expression evaluator then handles field access via dot notation (e.g. `status.phase`).

Nested paths with bracket notation (`.metadata.labels['app']`) are handled by a thin resolver in `internal/executor/resolver.go` that pre-processes the SELECT field list into octosql field descriptors.

**Alternative considered**: flatten all fields to a string map. Rejected — loses type information needed for WHERE comparisons (numbers, booleans, timestamps).

### Query execution flow

```
cmd/root.go
  └─ cobra.Args[0]  →  SQL string
       │
       ├─ internal/k8s/client.go
       │    NewDynamicClient(kubeconfig, context)  →  dynamic.Interface + RESTMapper
       │
       └─ internal/executor/executor.go
            KubernetesDataSource.Register(repo)
            octosql.RunQuery(sql, repo)  →  streams rows to stdout table
```

### Table output

Use octosql's built-in `output.NewTableOutput(os.Stdout)`. No custom renderer for this change — the `internal/output/renderer.go` stub remains empty until the `--output` flag is implemented.

## Risks / Trade-offs

- **Large result sets** → octosql buffers all rows before rendering the table. Mitigation: `--page-size` limits each LIST call; advise users to add `LIMIT` clause for large namespaces.
- **REST mapper cache staleness** → mapper is built once at startup; CRDs added after startup won't be visible. Mitigation: acceptable for a CLI tool; document as known limitation.
- **octosql SQL dialect** → octosql uses its own SQL subset; not all standard SQL is supported. Mitigation: document supported grammar; error messages from octosql are reasonably descriptive.
- **Unknown fields return NULL** → unstructured field access on missing paths returns `nil`; octosql maps this to NULL. This is the desired behaviour per AGENTS.md.

## Open Questions

- Should `metadata.name` and `metadata.namespace` be automatically aliased to `name` and `namespace` in the schema? (defer to a follow-up change)
