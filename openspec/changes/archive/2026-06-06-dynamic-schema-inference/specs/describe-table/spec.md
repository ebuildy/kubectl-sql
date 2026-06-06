## ADDED Requirements

### Requirement: DESCRIBE TABLE lists all columns and types for a resource
Running `DESCRIBE TABLE <resource>` SHALL return a two-column table (`COLUMN`, `TYPE`) listing every field that would appear in a `SELECT *` query for that resource, inferred from OpenAPI or a sample object.

#### Scenario: DESCRIBE TABLE pods lists expected columns
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"`
- **THEN** the output table contains rows for `name`, `namespace`, `status`, `spec`, and exits 0

#### Scenario: DESCRIBE TABLE works for any resource
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE configmaps"`
- **THEN** the output contains rows for `name`, `namespace`, and exits 0

#### Scenario: DESCRIBE TABLE on empty resource returns guaranteed columns
- **WHEN** the resource has no objects in the cluster
- **THEN** output contains at least `name`, `namespace` and exits 0

### Requirement: DESCRIBE TABLE is case-insensitive
The parser SHALL accept `describe table pods`, `DESCRIBE TABLE pods`, and `Describe Table Pods` as equivalent.

#### Scenario: Lowercase variant accepted
- **WHEN** the user runs `kubectl-sql "describe table pods"`
- **THEN** the command exits 0 and returns the column table
