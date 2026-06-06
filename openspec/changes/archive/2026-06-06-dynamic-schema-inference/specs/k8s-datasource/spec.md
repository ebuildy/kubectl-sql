## MODIFIED Requirements

### Requirement: Each resource object is exposed as a row
Each Kubernetes object returned by the LIST call SHALL be exposed as an octosql row. The row columns SHALL be derived from the OpenAPI schema or a sample object at query planning time. Guaranteed columns are `name` and `namespace`. Additional columns correspond to top-level keys (e.g. `status`, `spec`, `metadata`). Fields absent on a given object resolve to NULL.

### Requirement: Nested objects are exposed as octosql structs
Map values (e.g. `metadata`, `status`) SHALL be represented as `octosql.TypeIDStruct` with named fields inferred from the sample and/or OpenAPI schema. Nested struct access uses the `->` operator natively supported by octosql. Slices SHALL remain serialized as JSON strings.

### Requirement: No flattened underscore alias columns
There SHALL be no synthetic `metadata_labels`, `metadata_labels_app` style alias columns. Nested field access is expressed via `->` operator only. The dot-notation rewriter SHALL convert `metadata.labels.app` → `metadata->labels->app` before parsing.

#### Scenario: Top-level struct field access
- **WHEN** the user runs `SELECT metadata->labels FROM pods LIMIT 1` with `--output json`
- **THEN** the output contains `"app"` as a key in the struct value

#### Scenario: Deep struct field access
- **WHEN** the user runs `SELECT metadata->labels->app FROM pods`
- **THEN** the output contains `nginx`

#### Scenario: Dot notation is rewritten to arrow notation
- **WHEN** the user runs `SELECT metadata.labels.app FROM pods`
- **THEN** the query is rewritten to `metadata->labels->app` and returns `nginx`

#### Scenario: Missing field returns NULL
- **WHEN** the query selects a field path that does not exist on a resource
- **THEN** the value for that field is NULL (not an error)

#### Scenario: All top-level fields are accessible
- **WHEN** the user runs `SELECT * FROM pods`
- **THEN** columns include at minimum `name`, `namespace`, `status`, `spec`, `metadata`
