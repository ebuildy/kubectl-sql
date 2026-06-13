# Spec: Dynamic Schema Inference

## Purpose

Defines how `kubectl-sql` infers the schema of a Kubernetes resource at query planning time. Schema inference drives column discovery for `SELECT *`, `DESCRIBE TABLE`, and type-aware filtering.

---

## Requirements

### Requirement: Schema is inferred from OpenAPI primary, sample fallback
`GetTable` SHALL derive the schema by starting from a hardcoded **default baseline** and then layering enrichment fields on top. The default baseline SHALL always include the top-level fields `name`, `namespace`, `labels`, `annotations`, `metadata`, `spec`, and `status`. The inferrer SHALL then merge, in order: (1) the fields from an **embedded build-time swagger snapshot** for resources it covers, (2) the fields discovered from the cluster's live OpenAPI v3 document, and (3) the fields discovered from a sample object (a small LIST). Each layer enriches the previous one; later layers are not merely fallbacks for earlier ones. Unknown fields for any given row SHALL resolve to NULL.

When merging a source field list onto the destination tree:
- A field absent from the destination SHALL be appended.
- A field present in both whose types are equal and `object` SHALL be merged recursively into its subfields.
- A field present in both whose types disagree SHALL prefer the object form (enrichment): an object SHALL NOT be downgraded to a leaf, and a leaf SHALL be promoted to an object when the source is an object.
- A genuine leaf-vs-leaf type conflict (neither side an object) SHALL surface as a field-type-mismatch error to the orchestrator, which logs it and keeps the partial result.

#### Scenario: Default columns always present
- **WHEN** the user runs `SELECT * FROM pods`
- **THEN** the output table includes at least the columns `name`, `namespace`, `metadata`, `spec`, `status`

#### Scenario: Embedded swagger snapshot supplies full spec/status structure for standard resources
- **WHEN** the schema for `pods` (a resource covered by the embedded swagger snapshot) is inferred
- **THEN** `spec` and `status` are merged with their full nested structure (e.g. `spec->containers`, `status->phase`) from the embedded snapshot, even before the live OpenAPI v3 or sample layers run

#### Scenario: OpenAPI fields enrich the baseline
- **WHEN** the OpenAPI v3 schema for a resource exposes additional subfields under `spec`
- **THEN** those subfields are merged under the baseline `spec` object (as already enriched by the embedded snapshot, if covered) rather than replacing it

#### Scenario: Sample object supplies dynamic nested keys
- **WHEN** a sample pod carries `metadata.labels.app`
- **THEN** the sample layer is merged so `metadata->labels->app` resolves as a struct field

#### Scenario: Empty resource falls back to baseline
- **WHEN** the queried resource has no objects and no OpenAPI schema
- **THEN** the query returns an empty result with at least the default baseline columns and exits 0

#### Scenario: Resource not covered by the embedded snapshot is unaffected
- **WHEN** the schema for a CRD-backed resource (not present in the embedded swagger snapshot) is inferred
- **THEN** the embedded swagger layer contributes no fields, and the schema is derived exactly as before from the default baseline, live OpenAPI v3, and sample layers

---

### Requirement: Object columns use octosql TypeIDStruct
The schema model SHALL distinguish two object kinds:

- **Struct** (`FieldTypeObject`): a fixed, known schema such as `metadata`, `spec`, `status`. A struct has a bounded set of named subfields discoverable from OpenAPI. Maps that are also structs nest recursively as structs.
- **Map** (`FieldTypeMap`): an open-ended `map[string]T` such as `labels`, `annotations`. A map has a single declared value type and an unbounded, per-object set of keys.

Inference SHALL classify a field as a map when OpenAPI declares `type: object` with `additionalProperties` and no `properties` (or, for the default baseline, for the well-known fields `labels` and `annotations`); otherwise an object with `properties`/`$ref` is a struct. A field's kind, once set by the default baseline or OpenAPI, SHALL NOT be changed by a later sample layer.

A struct SHALL materialize as `octosql.TypeIDStruct` (so `->` field access works). A map SHALL materialize as `octosql.TypeIDList` whose element type is `octosql.Any` ("a typed map list"), holding that row's entries as a flat, alternating sequence `[key1, value1, key2, value2, ...]`. Each `keyN` SHALL be `octosql.String`; each `valueN` SHALL be the value's native octosql type (`Int`, `Float`, `Boolean`, or `String`), with nested objects/arrays falling back to a JSON-string element (matching how list-typed columns encode composite elements). octosql has no native map type and its Struct is a fixed positional shape, so a struct cannot represent per-row varying keys — a typed map list preserves both per-row keys and each value's native type. Slices (`FieldTypeList`) SHALL continue to be typed as `octosql.TypeIDList` with a `String` element type, distinguishing them from a map list (`Element` is `Any`) at the type level.

Map key access SHALL use bracket syntax `map['key']`, which the query rewriter lowers to `map_get(map, 'key')` (returns the key's value in its native type, or NULL if absent). `keys()`, `contains()`, `length()`, and `map_values()` SHALL operate on a map's flat key/value list (keys at even indices, values at odd indices), and JSON output SHALL render a map column as a JSON object with each value in its native JSON type (string, number, boolean).

#### Scenario: A fixed-schema field is a struct
- **WHEN** the schema is inferred for a pod's `metadata`
- **THEN** `metadata` is a struct (`FieldTypeObject`) with named subfields (`name`, `namespace`, …)

#### Scenario: An open-ended field is a map
- **WHEN** the schema is inferred for a pod's `metadata.labels`
- **THEN** `metadata.labels` is a map (`FieldTypeMap`), not a fixed-schema struct

#### Scenario: Map keys do not become top-level columns
- **WHEN** the user runs `DESCRIBE TABLE pods` or `SELECT * FROM pods`
- **THEN** `labels` and `annotations` each appear as a single column, and their per-object keys are NOT expanded into separate columns

#### Scenario: Each row keeps its own map keys
- **WHEN** two pods have different label sets and the user runs `SELECT metadata->labels FROM pods`
- **THEN** each row renders its own labels as a JSON object, with no keys dropped or invented across rows

#### Scenario: Map key access and helpers work
- **WHEN** the user runs `SELECT metadata.labels['app'], keys(metadata->labels), contains(metadata->labels, 'nginx'), length(metadata->labels) FROM pods WHERE name = 'nginx'`
- **THEN** `metadata.labels['app']` returns the key's value (NULL if absent), and `keys()`, `contains()`, and `length()` resolve against that row's flat key/value list

#### Scenario: map_get returns the value's native type
- **WHEN** a pod has an annotation whose value is an RFC3339 timestamp (e.g. `kubectl.kubernetes.io/restartedAt`)
- **THEN** `map_get(metadata->annotations, 'kubectl.kubernetes.io/restartedAt')` returns a `Time` value, not a JSON-escaped string, consistent with how other timestamp fields are typed

#### Scenario: A slice field is a list
- **WHEN** the schema is inferred for a pod with `spec.volumes`
- **THEN** `spec.volumes` is a list (`TypeIDList` with `Element` type `String`), not a struct or a map list (`Element` type `Any`)

---

### Requirement: No synthetic flattened alias columns
The schema SHALL NOT include synthetic `parent_child` underscore alias columns. All nested field access is performed via the `->` operator.

#### Scenario: Real resource fields appear as columns
- **WHEN** the user runs `SELECT * FROM pods`
- **THEN** the output table includes columns `name`, `namespace`, `status`, `spec`, `metadata`

#### Scenario: WHERE on nested struct field works
- **WHEN** the user runs `SELECT name FROM pods WHERE metadata->labels->app = 'nginx'`
- **THEN** the query executes without error and returns pods with label app=nginx

#### Scenario: Empty resource falls back to minimal schema
- **WHEN** the queried resource has no objects in the cluster
- **THEN** the query returns an empty result with at least `name`, `namespace` columns and exits 0
