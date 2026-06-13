# Spec: Swagger Schema Provider

## Purpose

Provides a build-time generated, embedded snapshot of the Kubernetes OpenAPI (`swagger.json`) schema for standard resources, so that `spec`/`status` field structure is available to the schema inferrer without any cluster round trip. A runtime loader decodes this embedded snapshot and serves it as an enrichment layer keyed by `GroupVersionResource`, falling through cleanly for resources it does not cover (e.g. CRDs).

---

## Requirements

### Requirement: Embedded swagger schema snapshot is generated at build time
The repository SHALL contain a generator (`tools/genk8sschema`) that converts a pinned `swagger.json` (Kubernetes OpenAPI v2 document, checked in under `internal/adapter/datasources/k8s/testdata/`) into a `map[string][]schema.Field` keyed by `"<group>/<version>/<resource>"` (empty group for the core API group, e.g. `"/v1/pods"`).

The generator SHALL include exactly the resources exposed via a `list` path that carries an `x-kubernetes-group-version-kind` extension, resolved to the matching definition by exact `(group, version, kind)` match. For each included resource, the generator SHALL recursively resolve `$ref` chains for object-typed properties (`spec`, `status`, and their nested fields) into `SubFields`, applying a fixed recursion depth cap and a cycle guard so self-referential definitions terminate. The output SHALL retain only `schema.Field{Name, Type, SubFields}` — descriptions, validation constraints, and all other OpenAPI metadata SHALL be discarded.

`make generate` SHALL run the generator against the pinned fixture and (re)produce `internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go`, a `// Code generated … DO NOT EDIT` file, deterministically (same input produces byte-identical output).

#### Scenario: Standard resource's spec and status are resolved to full depth
- **WHEN** the generator processes the `Pod` definition (`io.k8s.api.core.v1.Pod`, resource `pods`, group `""`, version `v1`)
- **THEN** the resulting `[]schema.Field` for key `"/v1/pods"` includes `spec` with nested object fields resolved through multiple `$ref` levels (e.g. `spec->affinity->nodeAffinity`) and `status` with nested `phase` and `conditions`, not just empty `object` fields. Array-typed fields such as `spec->containers` remain `FieldTypeList` leaves with no `SubFields`, consistent with the live OpenAPI inferrer.

#### Scenario: Self-referential definitions do not cause infinite recursion
- **WHEN** the generator encounters a definition that (directly or transitively) references itself, e.g. `JSONSchemaProps`-style validation schemas reachable from `CustomResourceDefinition`
- **THEN** generation completes without looping or stack overflow, and the self-referential branch is truncated to an object field with no further `SubFields`

#### Scenario: Resources without a list path or GVK extension are excluded
- **WHEN** a definition in `swagger.json` has no corresponding `list` path with an `x-kubernetes-group-version-kind` extension (e.g. a subresource-only or non-discoverable type)
- **THEN** that definition does not produce an entry in the generated map

#### Scenario: make generate is reproducible
- **WHEN** `make generate` is run twice against the same pinned `swagger.json` without other source changes
- **THEN** the generated file is byte-identical both times

---

### Requirement: Runtime loader serves the embedded schema by GroupVersionResource
A new `schemaInferrer` implementation, `swaggerSchemaProvider` (`internal/adapter/datasources/k8s/schema_swagger_loader.go`), SHALL lazily decompress and decode the embedded snapshot on first use and cache the result for subsequent calls. Given a `GroupVersionResource`, it SHALL look up `"<group>/<version>/<resource>"` in the decoded map.

On a match, `Provide` SHALL return `schema.GuaranteedFields()` followed by the stored fields for that resource. On no match (e.g. a CRD or any resource not present in the embedded snapshot), `Provide` SHALL return `(nil, nil)` — no error — so callers fall through to the live OpenAPI/sample layers unchanged.

#### Scenario: Standard resource returns its embedded field tree
- **WHEN** `swaggerSchemaProvider.Provide` is called with `GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}`
- **THEN** it returns the guaranteed fields followed by the embedded `pods` field tree (including nested `spec`/`status` structure) and a nil error

#### Scenario: Unknown resource returns no fields without error
- **WHEN** `swaggerSchemaProvider.Provide` is called with a `GroupVersionResource` not present in the embedded snapshot (e.g. a CRD resource)
- **THEN** it returns `(nil, nil)`

#### Scenario: Decoding happens at most once
- **WHEN** `swaggerSchemaProvider.Provide` is called multiple times across different resources
- **THEN** the embedded payload is decompressed and decoded only on the first call, and subsequent calls reuse the cached index
