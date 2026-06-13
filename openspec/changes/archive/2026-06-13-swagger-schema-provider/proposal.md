## Why

Schema inference for standard Kubernetes resources (pods, configmaps, deployments, services, …) currently gets its `spec`/`status` structure only from a live cluster: a per-resource OpenAPI v3 fetch (`openAPIInferrer`) and a sample `LIST` (`sampleInferrer`) layered on top of a near-empty hardcoded baseline (`defaultSchemaProvider`, which declares `spec`/`status` as opaque objects with no subfields). This means full field depth for `SELECT spec.containers[0].image FROM pods`-style queries, `--explain`, and `--dry-run` is unavailable until a cluster round trip succeeds, even though the structure of these well-known resources is static and published in the Kubernetes OpenAPI (`swagger.json`) spec. Pre-computing this structure at build time and embedding it removes that per-query cost and works without a live cluster.

## What Changes

- New build-time generator (`tools/genk8sschema`) that reads a pinned `swagger.json` (Kubernetes OpenAPI v2 spec) and, for every resource exposed via a `list` path carrying an `x-kubernetes-group-version-kind` extension, recursively resolves `$ref` chains into a `[]schema.Field` tree — full `spec`/`status` depth, not just one level — with cycle and depth guards for self-referential definitions (e.g. `JSONSchemaProps`).
- New generated file `internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go` (`// Code generated … DO NOT EDIT`) holding a gzip+gob-compressed `map[string][]schema.Field` keyed by `<group>/<version>/<resource>`. Only `schema.Field{Name, Type, SubFields}` is retained — descriptions and all other OpenAPI metadata are stripped, keeping the embedded payload small.
- New hand-written file `internal/adapter/datasources/k8s/schema_swagger_loader.go` implementing the existing `schemaInferrer` port: lazily (once) decompresses/decodes the embedded map and looks up the field tree for a given `GroupVersionResource`, returning `nil, nil` for resources it doesn't cover (e.g. CRDs).
- `strategicSchemaProvider.Provide` gains a new enrichment layer, inserted between the hardcoded default baseline and the live OpenAPI v3 layer: the embedded swagger schema fills in full `spec`/`status` structure for the ~129 standard resources it covers. The live OpenAPI and sample layers continue to run afterwards and still enrich/override (cluster-specific fields, CRDs, dynamic map keys).
- New `make generate` target that runs the generator against a pinned `swagger.json` fixture (checked into the repo under `internal/adapter/datasources/k8s/testdata/`) to (re)produce the generated resources file.
- `DESCRIBE TABLE <resource>` (`internal/domain/commands/query/command.go`) gains a third output column, `SCHEMA`: for any field whose type is `object` or `map` (`schema.FieldType.IsObjectLike()`) and that carries `SubFields` — now populated to full depth for standard resources via the embedded swagger snapshot — the column holds that field's full `SubFields` tree, JSON-encoded recursively. Fields with no `SubFields` (leaves, empty objects/maps, lists) leave the column blank. This is the primary user-facing way to inspect the depth the embedded snapshot adds to `spec`/`status`.

## Capabilities

### New Capabilities
- `swagger-schema-provider`: build-time generation of a compact, embedded `[]schema.Field` snapshot for standard Kubernetes resources from `swagger.json`, and a runtime loader that serves it as a schema-inference layer.

### Modified Capabilities
- `dynamic-schema`: the schema merge order gains the embedded swagger layer (default baseline → swagger snapshot → OpenAPI v3 → sample), so standard resources get full `spec`/`status` structure even before any cluster call succeeds.
- `describe-table`: `DESCRIBE TABLE` output gains a third `SCHEMA` column showing the full nested field tree (JSON-encoded) for `object`/`map` fields, surfacing the depth the embedded swagger snapshot now provides.

## Impact

- New files: `internal/adapter/datasources/k8s/schema_swagger_loader.go`, `internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go` (generated), `tools/genk8sschema/main.go` (+ its tests), pinned `internal/adapter/datasources/k8s/testdata/swagger.json`.
- Modified: `internal/adapter/datasources/k8s/schema.go` (`strategicSchemaProvider.Provide` — new layer, merge-order tests), `Makefile` (new `generate` target), `internal/domain/commands/query/command.go` (`runDescribeTable` — new `SCHEMA` column), `internal/port/schema/field.go` (JSON-encoding helper for a field's subtree).
- New tests: generator (`$ref` resolution, cycle guard, map-vs-struct classification, GVR key derivation from `paths`), loader (decompress/decode, lookup hit/miss), strategic provider (layer ordering with the new step), `runDescribeTable` (`SCHEMA` column populated for object/map fields with subfields, empty otherwise).
- No new runtime dependencies (gzip + gob + encoding/json are stdlib). No SQL grammar changes; `DESCRIBE TABLE` output format gains one column.
