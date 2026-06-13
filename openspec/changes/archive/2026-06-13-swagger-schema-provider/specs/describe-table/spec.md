## MODIFIED Requirements

### Requirement: DESCRIBE TABLE lists all columns and types for a resource
Running `DESCRIBE TABLE <resource>` SHALL return a three-column table (`COLUMN`, `TYPE`, `SCHEMA`) listing every field that would appear in a `SELECT *` query for that resource, inferred from the embedded swagger snapshot, live OpenAPI, or a sample object.

For each row, the `SCHEMA` column SHALL be populated only when the field's type is `object` or `map` (`schema.FieldType.IsObjectLike()`) AND the field has at least one subfield. In that case `SCHEMA` SHALL contain the field's full `SubFields` tree — recursively, to whatever depth was inferred — JSON-encoded as one object per field: `{"name":, "type":, "subFields":[...]}`, omitting `subFields` when a node has none. For all other fields (leaves, and `object`/`map` fields with no subfields), `SCHEMA` SHALL be empty.

#### Scenario: DESCRIBE TABLE pods lists expected columns
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"`
- **THEN** the output table contains rows for `name`, `namespace`, `status`, `spec`, and exits 0

#### Scenario: DESCRIBE TABLE works for any resource
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE configmaps"`
- **THEN** the output contains rows for `name`, `namespace`, `data`, and exits 0

#### Scenario: DESCRIBE TABLE on empty resource returns guaranteed columns
- **WHEN** the resource has no objects in the cluster
- **THEN** output contains at least `name`, `namespace` and exits 0

#### Scenario: Object field's SCHEMA column carries its full nested tree
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"` and the inferred `spec` field has `SubFields` (e.g. `containers`, `affinity`, populated by the embedded swagger snapshot)
- **THEN** the `spec` row's `SCHEMA` column contains valid JSON describing that nested tree, including entries for `containers` and `affinity`, recursed to their full depth

#### Scenario: Leaf and childless fields leave SCHEMA empty
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"`
- **THEN** the `name` row's `SCHEMA` column is empty, and any `object`/`map` field with no `SubFields` also has an empty `SCHEMA` column
