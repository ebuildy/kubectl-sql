# ADR-001: Schema Inference Strategy — OpenAPI Primary, Sample Fallback

**Date:** 2026-06-06  
**Status:** Accepted  
**Deciders:** Thomas Decaux

---

## Context

`kubectl-sql` needs a field schema for each Kubernetes resource at query planning time. octosql's typecheck phase requires a `physical.Schema` before any records are streamed. Two strategies were considered:

1. **Sample-object inference** — fetch one object via `LIST limit=1` and walk its keys
2. **OpenAPI/CRD schema** — fetch the cluster's OpenAPI document and parse the resource's schema

### The core problem with sample-only inference

The first object returned by `LIST limit=1` sets the schema for the entire query. Any field absent from that object is invisible to octosql at planning time — referencing it in `SELECT` or `WHERE` fails with "column not found":

```
Pod A (sample): { name, namespace, metadata, spec }         ← just created, no status yet
Pod B:          { name, namespace, metadata, spec, status }
Pod C:          { name, namespace, metadata, spec, status }

kubectl sql "SELECT name, status FROM pods"
→ typecheck error: column "status" not found
```

This is a fundamental limitation: the sample approach is probabilistic. It cannot guarantee schema completeness.

---

## Decision

Use **OpenAPI schema as primary, sample-object inference as fallback**.

The `SchemaInferrer` interface (port) abstracts the strategy. Two adapters are wired in priority order:

1. `OpenAPIInferrer` — fetches the cluster's OpenAPI v3 document, navigates to the resource schema, builds the field list. Used for built-in resources and CRDs that declare `openAPIV3Schema`.
2. `SampleInferrer` — fetches one object via `LIST limit=1`, walks its top-level keys. Used when OpenAPI returns no schema for the resource (unschematized CRDs, older clusters).

---

## Architecture

```
internal/schema/
  port.go              ← SchemaInferrer interface
  openapi_inferrer.go  ← OpenAPIInferrer (primary)
  sample_inferrer.go   ← SampleInferrer (fallback)
  composite_inferrer.go← CompositeInferrer: tries OpenAPI, falls back to Sample
  inferrer.go          ← shared field walking logic, Field/FieldType types
```

The executor and `cmd/root.go` depend only on `SchemaInferrer`. The default wiring at startup uses `CompositeInferrer`. Switching strategy requires only changing the wiring, not any consumer.

---

## Rationale

| Concern | OpenAPI primary | Sample fallback |
|---|---|---|
| **Schema completeness** | Complete for built-ins and schematized CRDs | Probabilistic — misses fields absent on first object |
| **CRD gaps** | Falls back gracefully | Works for all CRDs regardless of schema presence |
| **Latency** | ~100–200 ms (OpenAPI doc, cached after first fetch) | ~10–20 ms per query |
| **Type accuracy** | Exact types from spec | Runtime types only — no array element types |
| **Complexity** | Higher | ~50 lines |

OpenAPI is the right primary source: it is authoritative, complete, and solves the missing-field problem entirely for built-in resources (pods, deployments, nodes, etc.) which are the most common query targets.

---

## Consequences

- First query against a resource may be slightly slower due to OpenAPI doc fetch. Subsequent queries benefit if the doc is cached.
- CRDs without `openAPIV3Schema` still use sample inference and retain the probabilistic limitation — this is acceptable since unschematized CRDs are uncommon in modern clusters.
- Array element types remain unresolvable from sample inference. OpenAPI provides them correctly for built-in resources.
- The `raw` field always provides a full JSON escape hatch regardless of which inferrer ran.

---

## Future

- Cache the OpenAPI document per cluster connection to eliminate per-query latency.
- Add a `--schema-strategy` flag (`openapi`, `sample`, `auto`) for explicit user control.
