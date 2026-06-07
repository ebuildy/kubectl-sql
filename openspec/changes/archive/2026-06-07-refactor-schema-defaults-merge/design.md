## Context

Schema inference for the k8s datasource lived in a single `inferrer.go` file and followed an "OpenAPI primary, sample fallback" model where the two sources were alternatives, never combined into a stable baseline. This made `SELECT *` / `DESCRIBE TABLE` column sets depend on whatever the cluster happened to return, and partial/empty clusters could omit well-known top-level fields. The work is already underway on branch `fix_schema_claude_shit`: the inferrer has been split into focused files and a `mergeSchemas` routine added. This change documents and finalizes that refactor.

## Goals / Non-Goals

**Goals:**
- Always produce a predictable schema baseline (`name`, `namespace`, `labels`, `annotations`, `metadata`, `spec`, `status`).
- Enrich, not replace, that baseline with OpenAPI v3 fields, falling back to a sample object when OpenAPI is empty.
- Merge recursively so object subfields combine instead of clobbering.
- Reject contradictory field types across sources instead of silently overwriting.
- Keep `make lint build` and the full `go test ./...` suite green.

**Non-Goals:**
- Changing the `->` nested access operator or the no-synthetic-alias-columns rule.
- Adding caching or new external dependencies.
- Adding any write/patch/exec paths (plugin stays read-only).

## Decisions

**Defaults-first composite.** `strategicSchemaProvider` builds a `root` object field from `defaultSchemaProvider`, then merges the OpenAPI result (or sample fallback) into it via `mergeSchemas`. Alternative considered: keep OpenAPI/sample as the base and append defaults — rejected because it made the guaranteed column set non-deterministic and harder to reason about.

**Recursive merge by name with type-conflict as error.** `mergeSchemas(root, fields)` indexes `root.SubFields` by name; matching object fields recurse, new fields append, and a type disagreement returns an error. Alternative considered: last-writer-wins — rejected because it can corrupt the baseline (e.g. an array source field overwriting an object).

**File split.** `schema.go` (orchestration + merge), `schema_default.go`, `schema_openapi.go`, `schema_sample.go`; old `inferrer.go`/`inferrer_test.go` removed. Keeps each source's logic isolated and testable.

## Risks / Trade-offs

- [Type-mismatch error aborts inference for the whole resource] → On error, orchestration logs and returns the partial `root.SubFields` (baseline still present) rather than failing the query.
- [Dead commented-out code left in `schema.go`] → Remove the legacy commented merge block during apply to keep the file clean and lint-friendly.
- [Default baseline drifts from real k8s shape] → Baseline is intentionally shallow (top-level objects only); OpenAPI/sample supplies depth, so drift risk is low.

## Migration Plan

No data migration. Behavioral change is additive (more columns guaranteed). Rollback = revert the branch; the previous "OpenAPI primary, sample fallback" path is preserved in git history.
