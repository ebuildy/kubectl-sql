## MODIFIED Requirements

### Requirement: Embedded swagger schema snapshot is generated at build time
The repository SHALL contain a generator (`tools/genk8sschema`) that converts a pinned `swagger.json` (Kubernetes OpenAPI v2 document, checked in under `internal/adapter/datasources/k8s/testdata/`) into a `map[string][]schema.Field` keyed by `"<group>/<version>/<resource>"` (empty group for the core API group, e.g. `"/v1/pods"`).

The generator SHALL include exactly the resources exposed via a `list` path that carries an `x-kubernetes-group-version-kind` extension, resolved to the matching definition by exact `(group, version, kind)` match. For each included resource, the generator SHALL recursively resolve `$ref` chains for object-typed properties (`spec`, `status`, and their nested fields) into `SubFields`, applying a fixed recursion depth cap and a cycle guard so self-referential definitions terminate.

For **array-typed** properties whose element schema (the array's `items`) is, or `$ref`s, an object, the generator SHALL resolve the element's object fields into the list field's `SubFields` — under the same depth cap and cycle guard. On a `FieldTypeList` field, `SubFields` describe the schema of each list **element** (not subfields of the list itself). Array properties whose element is a scalar, a map, or has no resolvable object schema SHALL remain `FieldTypeList` leaves with no `SubFields`.

An **open-ended map** property — declared with `additionalProperties` and no fixed `properties` (a `map[string]T` such as `metadata->labels`, `metadata->annotations`, or `spec->nodeSelector`) — SHALL be classified as `FieldTypeMap` with no `SubFields`, at every nesting depth, including when it appears inside a resolved list element or a nested struct. It SHALL NOT be emitted as a childless `FieldTypeObject`.

The output SHALL retain only `schema.Field{Name, Type, SubFields}` — descriptions, validation constraints, and all other OpenAPI metadata SHALL be discarded.

`make generate` SHALL run the generator against the pinned fixture and (re)produce `internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go`, a `// Code generated … DO NOT EDIT` file, deterministically (same input produces byte-identical output).

#### Scenario: Standard resource's spec and status are resolved to full depth
- **WHEN** the generator processes the `Pod` definition (`io.k8s.api.core.v1.Pod`, resource `pods`, group `""`, version `v1`)
- **THEN** the resulting `[]schema.Field` for key `"/v1/pods"` includes `spec` with nested object fields resolved through multiple `$ref` levels (e.g. `spec->affinity->nodeAffinity`) and `status` with nested `phase` and `conditions`, not just empty `object` fields

#### Scenario: Array element object schema is resolved into list SubFields
- **WHEN** the generator processes the `Pod` definition and reaches the array property `spec->containers` whose items `$ref` `io.k8s.api.core.v1.Container`
- **THEN** the resulting `spec->containers` field is `FieldTypeList` with `SubFields` describing a Container element (e.g. `name`, `image`, `ports`), so the element schema is available to callers

#### Scenario: Scalar-element arrays remain childless list leaves
- **WHEN** the generator reaches an array property whose items are a scalar (e.g. a `[]string` such as a container command/args list)
- **THEN** that field is `FieldTypeList` with no `SubFields`

#### Scenario: Open-ended map properties are classified as maps at any depth
- **WHEN** the generator resolves a property declared with `additionalProperties` and no fixed `properties` (e.g. `metadata->labels`, `metadata->annotations`, `spec->nodeSelector`), including such a property nested inside a resolved list element (e.g. a Container `resources->limits`)
- **THEN** that field is `FieldTypeMap` with no `SubFields`, not a childless `FieldTypeObject`

#### Scenario: Self-referential definitions do not cause infinite recursion
- **WHEN** the generator encounters a definition that (directly or transitively) references itself, e.g. `JSONSchemaProps`-style validation schemas reachable from `CustomResourceDefinition`
- **THEN** generation completes without looping or stack overflow, and the self-referential branch is truncated to an object field with no further `SubFields`

#### Scenario: Resources without a list path or GVK extension are excluded
- **WHEN** a definition in `swagger.json` has no corresponding `list` path with an `x-kubernetes-group-version-kind` extension (e.g. a subresource-only or non-discoverable type)
- **THEN** that definition does not produce an entry in the generated map

#### Scenario: make generate is reproducible
- **WHEN** `make generate` is run twice against the same pinned `swagger.json` without other source changes
- **THEN** the generated file is byte-identical both times
