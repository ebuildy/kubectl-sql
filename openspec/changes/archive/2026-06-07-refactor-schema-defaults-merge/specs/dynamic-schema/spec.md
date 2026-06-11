## MODIFIED Requirements

### Requirement: Schema is inferred from OpenAPI primary, sample fallback
`GetTable` SHALL derive the schema by starting from a hardcoded **default baseline** and then layering cluster-derived fields on top. The default baseline SHALL always include the top-level fields `name`, `namespace`, `labels`, `annotations`, `metadata`, `spec`, and `status`. The inferrer SHALL then merge, in order, the fields discovered from the OpenAPI v3 document and the fields discovered from a sample object (a small LIST). Both layers enrich the baseline; the sample layer is not merely a fallback. Unknown fields for any given row SHALL resolve to NULL.

When merging a source field list onto the baseline tree:
- A field absent from the destination SHALL be appended.
- A field present in both whose types are equal and `object` SHALL be merged recursively into its subfields.
- A field present in both whose types disagree SHALL prefer the object form (enrichment): an object SHALL NOT be downgraded to a leaf, and a leaf SHALL be promoted to an object when the source is an object.
- A genuine leaf-vs-leaf type conflict (neither side an object) SHALL surface as a field-type-mismatch error to the orchestrator, which logs it and keeps the partial result.

#### Scenario: Default columns always present
- **WHEN** the user runs `SELECT * FROM pods`
- **THEN** the output table includes at least the columns `name`, `namespace`, `metadata`, `spec`, `status`

#### Scenario: OpenAPI fields enrich the baseline
- **WHEN** the OpenAPI v3 schema for a resource exposes additional subfields under `spec`
- **THEN** those subfields are merged under the baseline `spec` object rather than replacing it

#### Scenario: Sample object supplies dynamic nested keys
- **WHEN** a sample pod carries `metadata.labels.app`
- **THEN** the sample layer is merged so `metadata->labels->app` resolves as a struct field

#### Scenario: Empty resource falls back to baseline
- **WHEN** the queried resource has no objects and no OpenAPI schema
- **THEN** the query returns an empty result with at least the default baseline columns and exits 0

### Requirement: Object columns use octosql TypeIDStruct
Top-level map fields (e.g. `metadata`, `status`, `spec`) SHALL be typed as `octosql.TypeIDStruct` with named subfields. Nested subfields that are also maps SHALL be recursively typed as `TypeIDStruct`. Slices SHALL be typed as `octosql.TypeIDList` with JSON-string elements, so `length()` counts elements and the column renders as a JSON array.

#### Scenario: A map field is a struct
- **WHEN** the schema is inferred for a pod with `metadata.labels`
- **THEN** `metadata.labels` is a struct (`TypeIDStruct`) with a named subfield per label key

#### Scenario: A slice field is a list
- **WHEN** the schema is inferred for a pod with `spec.volumes`
- **THEN** `spec.volumes` is a list (`TypeIDList`), not a struct

#### Scenario: length() counts list elements and struct fields
- **WHEN** the user runs `SELECT length(metadata->labels), length(spec->volumes) FROM pods WHERE name = 'nginx'`
- **THEN** `length(metadata->labels)` returns the number of labels and `length(spec->volumes)` returns the number of volumes
