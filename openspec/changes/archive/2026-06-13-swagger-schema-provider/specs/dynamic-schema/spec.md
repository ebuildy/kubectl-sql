## MODIFIED Requirements

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
