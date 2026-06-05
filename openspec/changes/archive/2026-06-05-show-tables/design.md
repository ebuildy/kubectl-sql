## Context

`SHOW TABLES` is a MySQL-style introspection command. octosql does not support it natively — it only handles SELECT. The command must be intercepted before the octosql pipeline and handled directly.

The REST mapper already holds all discovered API group resources (built during `NewDynamicClient`). We can iterate it to list all resource types without additional API calls.

## Goals / Non-Goals

**Goals:**
- Intercept `SHOW TABLES` (case-insensitive) in `cmd/root.go` before `rewriteQuery` and octosql
- Use the existing REST mapper to enumerate all API resources
- Output a table with columns: `name`, `group`, `version`, `namespaced`
- Use `tablewriter` (already a dependency) for consistent formatting
- Add e2e scenario (no-cluster, just checks exit 0 and column headers are present)
- Add integration scenario against envtest cluster

**Non-Goals:**
- Supporting `SHOW TABLES LIKE` or filtering
- Caching the resource list between calls

## Decisions

### Intercept in cmd/root.go before octosql

`SHOW TABLES` is not valid SQL — passing it to `sqlparser.Parse` returns an error. Detection must happen before `rewriteQuery`. A simple `strings.EqualFold(strings.TrimSpace(query), "show tables")` check is sufficient.

### Use RESTMapper for resource enumeration

`meta.RESTMapper` has no direct "list all resources" method. We use the underlying `restmapper.GetAPIGroupResources` data already fetched in `NewDynamicClient`. To expose it, `KubernetesDatabase.ListTables` (currently returns nil) is implemented to walk the mapper and return resource metadata.

Simpler alternative: re-call `restmapper.GetAPIGroupResources(discoClient)` directly in the `SHOW TABLES` handler. This avoids threading the raw group resources through the client layer and keeps `cmd/root.go` as the integration point. **Chosen approach** — lower coupling.

### Output format

Use `tablewriter` directly (same as octosql's formatter) with columns `NAME`, `GROUP`, `VERSION`, `NAMESPACED`. Sort by group then name for deterministic output.

### `namespaced` field

`meta.RESTMapper` does not expose the namespaced flag directly. We use `discovery.ServerPreferredResources` via the discovery client to get `APIResource.Namespaced`. This requires passing the discovery client to the show-tables handler.

Alternative: use `mapper.ResourceFor` on each resource and check scope. **Rejected** — too many round trips.

Simpler alternative: omit the `namespaced` column for now. **Chosen** — keeps implementation minimal. Can be added later.

### New function signature

```go
// in cmd/root.go
func runShowTables(dynClient dynamic.Interface, mapper meta.RESTMapper) error
```

Iterates `mapper.(*restmapper.ShortcutExpander)` — but the mapper is typed as `meta.RESTMapper` (interface). Instead, re-call `discovery.NewDiscoveryClientForConfig` + `restmapper.GetAPIGroupResources` inside the handler since we already have `cfg`.

Cleaner: pass `discoClient discovery.DiscoveryInterface` through from `NewDynamicClient`. Update signature to return triple `(dynamic.Interface, meta.RESTMapper, discovery.DiscoveryInterface, error)`.

## Components

```
cmd/root.go
  runQuery()
    ├── if strings.EqualFold(trimmed, "show tables")
    │     └── runShowTables(discoClient) → print table → return
    └── else → octosql pipeline (unchanged)

internal/k8s/client.go
  NewDynamicClient() → returns (dynamic.Interface, meta.RESTMapper, discovery.DiscoveryInterface, error)

internal/executor/executor.go  (no changes needed)
```

## Risks

- `ServerPreferredResources` makes additional API calls — acceptable since `SHOW TABLES` is an explicit introspection command, not part of a query.
- Large clusters may have many CRDs — output could be long. Acceptable; no truncation.
