# Spec: Dynamic Schema Inference

## Purpose

Defines how `kubectl-sql` infers the schema of a Kubernetes resource at query planning time. Schema inference drives column discovery for `SELECT *`, `DESCRIBE TABLE`, and type-aware filtering.

---

## Requirements

### Requirement: Schema is inferred from OpenAPI primary, sample fallback
`GetTable` SHALL derive the schema via a `SchemaInferrer` — OpenAPI v3 as primary, sample object (1-item LIST) as fallback. The schema SHALL always include `name` and `namespace` as guaranteed fields. Unknown fields for any given row SHALL resolve to NULL.

---

### Requirement: Object columns use octosql TypeIDStruct
Top-level map fields (e.g. `metadata`, `status`, `spec`) SHALL be typed as `octosql.TypeIDStruct` with named subfields. Nested subfields that are also maps SHALL be recursively typed as `TypeIDStruct`. Slices SHALL be typed as `octosql.String` (JSON-serialized).

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
