## Context

`strategicSchemaProvider.Provide` (`internal/adapter/datasources/k8s/schema.go`) currently layers three sources onto a hardcoded baseline (`schema_default.go`):

1. `defaultSchemaProvider` — `name`, `namespace`, `labels`, `annotations`, `metadata` (with subfields), and bare `spec`/`status` objects with **no** subfields.
2. `openAPIInferrer` (`schema_openapi.go`) — fetches the cluster's live OpenAPI v3 document per GVR and converts `spec.Schema` properties to `[]schema.Field`, two levels deep.
3. `sampleInferrer` (`schema_sample.go`) — LISTs up to 50 objects and unions `schema.InferFields` over them for dynamic map keys.

All three require a reachable cluster. For the ~129 standard resources Kubernetes ships (pods, configmaps, deployments, services, …), the `spec`/`status` shape is fully described by the Kubernetes OpenAPI v2 document (`swagger.json`, available at `internal/adapter/datasources/k8s/testdata/swagger.json` once pinned — currently a 4 MB untracked file at the repo root). That shape is static per Kubernetes version and can be converted to `[]schema.Field` once, at build time, the same way `schema_openapi.go` already converts the *live* OpenAPI v3 schema — except recursing through `$ref`s instead of stopping at depth 2.

`k8s.io/kube-openapi/pkg/validation/spec` (already an indirect dependency via `k8s.io/client-go`, and already imported by `schema_openapi.go`) provides `spec.Swagger`, which unmarshals an OpenAPI v2 document directly: `SwaggerProps.Definitions` (`map[string]Schema`) and `SwaggerProps.Paths`. Both schema objects and path operations carry `VendorExtensible.Extensions`, giving typed access to `x-kubernetes-group-version-kind` and `x-kubernetes-action`. No new dependency is needed.

## Goals / Non-Goals

**Goals:**
- Build-time generator that turns a pinned `swagger.json` into a compact, embedded `map[string][]schema.Field]` covering every resource Kubernetes exposes via a `list` path.
- A runtime loader (`schemaInferrer`) that serves that map by GVR with zero I/O and near-zero cost after first use.
- Wire the loader into `strategicSchemaProvider` as a pure enrichment layer — never removes or downgrades fields the other layers would have produced.
- Keep the embedded payload small by retaining only `schema.Field{Name, Type, SubFields}` (descriptions, validation rules, etc. from `swagger.json` are dropped).
- `make generate` reproduces the generated file deterministically from the pinned fixture.

**Non-Goals:**
- No change to whether/when the live OpenAPI v3 or sample layers run — this change is strictly additive; skipping the live fetch for covered resources is a possible future optimization, not part of this change.
- No coverage for CRDs or aggregated APIs — they have no entry in `swagger.json` and continue to rely on the live OpenAPI/sample layers exactly as today.
- No new CLI flags, SQL grammar, or output-format changes.
- The generator is a dev-time tool (`go run ./tools/genk8sschema`, via `make generate`), not a user-facing subcommand.

## Decisions

1. **Parse `swagger.json` with `k8s.io/kube-openapi/pkg/validation/spec.Swagger`.** It already models OpenAPI v2 `definitions` and `paths` with the same `Schema`/`Extensions` types `schema_openapi.go` already uses for OpenAPI v3, so type-conversion logic (`openAPITypeToFieldType`-style map/struct/list classification) can be shared/mirrored without a new dependency.
   - *Alternative considered*: hand-rolled `map[string]interface{}` walking via `encoding/json` — rejected, loses typed access to `additionalProperties`/`$ref` and duplicates what `spec.Schema` already gives us.

2. **Discover resources from `paths`, not by guessing pluralization.** For every path entry whose `get` operation has `Extensions["x-kubernetes-action"] == "list"`, read `Extensions["x-kubernetes-group-version-kind"]` (group, version, kind) and take the path's last segment as the resource name (e.g. `/apis/apps/v1/namespaces/{namespace}/deployments` → `deployments`). This yields exact `(group, version, resource) → (group, version, kind)` tuples for all ~129 list-able resources without a singular/plural mapping table.
   - *Alternative considered*: deriving the resource name from the Kind via pluralization rules (`Pod`→`pods`, `Endpoints`→`endpoints`, `NetworkPolicy`→`networkpolicies`, …) — rejected, Kubernetes pluralization has enough irregulars that a rule table would need ongoing maintenance; the `paths` already encode the ground truth.

3. **Resolve each resource's definition by exact GVK match.** Build an index from `swagger.Definitions`: for each definition carrying `x-kubernetes-group-version-kind`, map `(group, version, kind)` → definition name (e.g. `("". "v1", "Pod")` → `io.k8s.api.core.v1.Pod`). List-wrapper types (`PodList`, kind `"PodList"`) have a different `kind` and are naturally excluded by the exact match.

4. **Recursively resolve `$ref` into `[]schema.Field`, with a depth cap and cycle guard.** A new `defToFields(defName string, depth int, visiting map[string]bool) []schema.Field` mirrors `openAPISchemaToField`/`openAPITypeToFieldType` (map vs. struct vs. list classification) but recurses into `$ref`ed definitions for object-typed properties instead of stopping after one level. `visiting` tracks definition names on the current recursion path; revisiting one (e.g. `JSONSchemaProps` → `additionalProperties` → `JSONSchemaProps`) or exceeding a fixed depth (8) truncates that branch to an object field with no subfields — `->` access below that point resolves to `NULL` per existing resolver rules, which is acceptable for the rarely-queried recursive validation schemas.
   - *Alternative considered*: unlimited recursion — rejected, `JSONSchemaProps` and similar self-referential definitions would recurse forever.
   - *Alternative considered*: skip self-referential resources entirely — rejected, only the *self-referential branch* is truncated; the rest of the resource (e.g. `CustomResourceDefinition.spec.versions[].name`) stays useful.

5. **Embed the data via `//go:embed` of a gzip+gob binary asset, not a `[]byte{...}` Go literal.** The generator writes two artifacts together: `internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go` (`// Code generated … DO NOT EDIT`, ~10 lines: package decl, `//go:embed schema_swagger_k8s_standard_resources.bin.gz`, `var swaggerSchemaDataGzip []byte`) and the sibling `.bin.gz` asset (gzip-compressed `encoding/gob` of `map[string][]schema.Field`, keyed by `"<group>/<version>/<resource>"`, empty group for core e.g. `"/v1/pods"`). `gob` needs no registration for `schema.Field` (concrete exported struct, no interfaces).
   - *Alternative considered*: a base64 string constant or `[]byte{0x1f, 0x8b, ...}` literal inside the `.go` file only — rejected, multi-hundred-KB literals bloat diffs and slow `gofmt`/compile for no benefit; `//go:embed` (stdlib since Go 1.16) is exactly this use case and AGENTS.md's "no dependencies without justification" guardrail is satisfied trivially (stdlib only).

6. **New `schemaInferrer`: `swaggerSchemaProvider` in `schema_swagger_loader.go`.** Decompresses + `gob`-decodes the embedded map once via `sync.Once` into a package-level index. `Provide(ctx, gvr)` builds the key `gvr.Group + "/" + gvr.Version + "/" + gvr.Resource`, looks it up, and on a hit returns `schema.GuaranteedFields()` + the stored fields (mirroring `openAPIInferrer.Provide`'s shape); on a miss returns `nil, nil` so the caller falls through to the live layers unchanged.

7. **Insert the new layer between the default baseline and the live OpenAPI v3 layer in `strategicSchemaProvider.Provide`.** Order becomes: default baseline → **embedded swagger snapshot** → live OpenAPI v3 → sample. `mergeSchemas` already treats every layer as enrichment (object fields are merged recursively, leaf-vs-object promotes to object, leaf-vs-leaf conflicts error and are logged) — no change to `mergeSchemas` itself. Running the swagger layer first means a live cluster on a different Kubernetes version can still add/override fields the pinned snapshot doesn't have; it only ever adds structure the baseline was missing.

8. **Generator location and `make generate`.** New `tools/genk8sschema/main.go` (`package main`), invoked as `go run ./tools/genk8sschema -in <swagger.json> -out <generated.go>`. New Makefile target:
   ```makefile
   generate:
       go run ./tools/genk8sschema -in internal/adapter/datasources/k8s/testdata/swagger.json \
           -out internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go
   ```
   The pinned `swagger.json` (Kubernetes OpenAPI v2 spec for a specific release, currently sitting untracked at the repo root) is moved to `internal/adapter/datasources/k8s/testdata/swagger.json` and checked in; a header comment in the generated file records the source Kubernetes version for future regeneration.

9. **`DESCRIBE TABLE`'s new `SCHEMA` column is a field's `SubFields` tree, JSON-encoded via a new `schema` helper.** A new function `schema.MarshalSubFieldsJSON(fields []Field) (string, error)` (in `internal/port/schema/field.go`, alongside the existing library-free `Field`/`FieldType` types — `encoding/json` is stdlib, consistent with that file's "no external libs" intent) recursively converts `[]Field` to a minimal JSON shape — `{"name":, "type":, "subFields":[...]}` per node, omitting `Path` (an internal resolver detail) and omitting `subFields` entirely when empty. `runDescribeTable` calls it for each top-level field where `f.Type.IsObjectLike() && len(f.SubFields) > 0`, and puts the result in the new `SCHEMA` cell; all other fields (leaves, empty objects/maps, lists) get an empty `SCHEMA` cell.
   - *Alternative considered*: `json.Marshal` directly on `[]schema.Field` without a helper — rejected, the zero-value `Path` field and Go-exported `Name`/`Type`/`SubFields` keys would leak resolver-internal detail and produce noisier JSON than necessary for a human-readable table cell.
   - *Alternative considered*: restrict the new column to `FieldTypeObject` only, excluding `FieldTypeMap` — rejected, `IsObjectLike()` is the existing distinction for "has named subfields and nests recursively"; excluding `map` would hide the sample-inferred subtree of fields like `metadata.labels` for no benefit.
   - *Alternative considered*: show only one level of subfields — rejected, the goal is the *full* schema; one level would leave the embedded swagger snapshot's main contribution (full `spec`/`status` depth) invisible in `DESCRIBE TABLE`.

## Risks / Trade-offs

- [Embedded snapshot is pinned to one Kubernetes version and can drift from the cluster's actual version] → mitigated by treating it as enrichment only: the live OpenAPI/sample layers still run and can add fields the snapshot lacks; core resource shapes (Pod, ConfigMap, Deployment, …) are stable across versions, so drift mainly affects newly-added fields on newer clusters, which the live layer supplies.
- [Generated asset size] → gzip+gob of `[]schema.Field` (no descriptions, no validation metadata) for ~129 resources is expected to be tens to ~a couple hundred KB; verified during implementation and called out in the PR if larger than expected.
- [`schema.Field`'s shape changing later requires regenerating the embedded asset] → the generated file carries a `// Code generated … DO NOT EDIT` header per AGENTS.md guardrail #4 and is reproduced via `make generate`; a follow-up that changes `schema.Field` must re-run `make generate` (called out in that change's tasks, not enforced by CI in this change).
- [`$ref` cycles in less-common definitions] → depth cap (8) + `visiting` cycle guard truncate the branch to a childless object rather than recursing forever; covered by a generator unit test using a small fixture with a deliberate self-reference.
- [129 list-able resources is a lot to eyeball] → resource discovery and definition resolution are both mechanical (`paths` → GVK → `definitions` index), so adding/removing resources only requires re-pinning `swagger.json`, not code changes.
- [`SCHEMA` column can be very wide/long for deeply-nested resources (`pods`, `deployments`, …) now that the embedded snapshot supplies full `spec`/`status` depth] → no truncation is added for this column since its purpose is to show the *full* tree on demand; `runDescribeTable` calls `table.SetAutoWrapText(false)` so the JSON stays on one line and valid/copyable rather than being word-wrapped (`tablewriter`'s default 30-char wrap would otherwise break mid-token). Users wanting compact output continue to use `SELECT * FROM pods` (unaffected by this change).
