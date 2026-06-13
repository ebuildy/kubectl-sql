# Spec: DESCRIBE TABLE

## Purpose

Defines the behavior of the `DESCRIBE TABLE <resource>` SQL command, which lists all columns and their types for a given Kubernetes resource. Column information is inferred from OpenAPI or a sample object.

---

## Requirements

### Requirement: DESCRIBE TABLE lists all columns and types for a resource
Running `DESCRIBE TABLE <resource>` SHALL return a three-column table (`COLUMN`, `TYPE`, `SCHEMA`) listing fields inferred from the embedded swagger snapshot, live OpenAPI, or a sample object, as follows:

- For each top-level (depth-1) field `f`:
  - If `f`'s type is `object` or `map` (`schema.FieldType.IsObjectLike()`) AND `f` has at least one subfield, `f` itself SHALL NOT get its own row. Instead, one row SHALL be emitted per immediate subfield `sf`, with `COLUMN` set to `f.Name->sf.Name` (e.g. `metadata->name`, `metadata->labels`, `status->conditions`).
  - Otherwise (leaf fields, and `object`/`map` fields with no subfields), `f` gets a single row with `COLUMN` set to `f.Name`.

For each emitted row (whether a depth-1 field or a depth-2 `parent->child` field), `SCHEMA` SHALL be populated only when that row's field type is `object` or `map` AND it has at least one subfield. In that case `SCHEMA` SHALL contain that field's `SubFields` tree, recursively up to a fixed depth limit, JSON-encoded as one object per field: `{"name":, "type":, "subFields":[...]}`, omitting `subFields` for nodes beyond the depth limit or with none. For all other rows, `SCHEMA` SHALL be empty.

`SCHEMA` JSON SHALL be pretty-printed (multi-line, 2-space indent). When the output destination is a terminal AND neither `--no-color` nor `--disable-beauty` is set, the JSON object keys (`"name"`, `"type"`, `"subFields"`) in `SCHEMA` SHALL be rendered in ANSI cyan, matching the "beauty" coloring applied to struct/map cells in query results.

#### Scenario: DESCRIBE TABLE pods lists expected columns
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"`
- **THEN** the output table contains rows for `name`, `namespace`, and depth-2 fields such as `status->phase` and `spec->containers`, and exits 0

#### Scenario: DESCRIBE TABLE works for any resource
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE configmaps"`
- **THEN** the output contains rows for `name`, `namespace`, `data`, and exits 0

#### Scenario: DESCRIBE TABLE on empty resource returns guaranteed columns
- **WHEN** the resource has no objects in the cluster
- **THEN** output contains at least `name`, `namespace` and exits 0

#### Scenario: Object fields are expanded into depth-2 "parent->child" rows
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"` and the inferred `metadata` field has `SubFields` including `name` and `labels`
- **THEN** the output contains rows `metadata->name` and `metadata->labels`, but no standalone `metadata` row

#### Scenario: A depth-2 field's SCHEMA column carries its pretty-printed nested tree
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"` and the inferred `metadata->labels` field has its own `SubFields` (e.g. sample label keys)
- **THEN** the `metadata->labels` row's `SCHEMA` column contains valid, multi-line indented JSON describing those subfields, recursed up to the configured depth limit

#### Scenario: Leaf and childless fields leave SCHEMA empty
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"`
- **THEN** the `name` row's `SCHEMA` column is empty, and any `object`/`map` field with no `SubFields` also has an empty `SCHEMA` column

#### Scenario: SCHEMA JSON keys are colorized in beauty mode
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"` with output to a terminal and without `--no-color` or `--disable-beauty`
- **THEN** the `"name"`, `"type"`, and `"subFields"` keys in non-empty `SCHEMA` cells are wrapped in ANSI cyan escape codes

---

### Requirement: DESCRIBE TABLE rows follow a fixed well-known field order

Since every Kubernetes object shares the same overall shape, rows SHALL be ordered using the following fixed priority list, matched against each field's own name (`f.Name` for depth-1 rows, `sf.Name` for the child half of a depth-2 `parent->child` row):

```
apiVersion, kind, metadata, name, namespace, annotations, labels, spec, data, stringData, status
```

Fields whose name is not in this list SHALL keep their existing relative (inferred) order and SHALL be listed after all fields that do appear in the list. This ordering SHALL be applied independently at each level: once to the top-level fields, and once to each expanded object/map field's immediate subfields (e.g. `metadata`'s subfields are ordered the same way, producing `metadata->name`, `metadata->namespace`, `metadata->annotations`, `metadata->labels` in that order).

#### Scenario: Well-known fields appear in the fixed order
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"`
- **THEN** rows derived from `metadata`, `name`, `namespace`, `annotations`, `labels`, `spec`, and `status` appear in that relative order, regardless of the order returned by schema inference

#### Scenario: metadata's subfields follow the same order
- **WHEN** the user runs `kubectl-sql "DESCRIBE TABLE pods"` and `metadata` has subfields `name`, `namespace`, `labels`, `annotations`
- **THEN** the output lists `metadata->name`, `metadata->namespace`, `metadata->annotations`, `metadata->labels` in that order

#### Scenario: Unlisted fields are appended after well-known fields
- **WHEN** a resource has custom top-level fields not present in the fixed order list (e.g. a CRD field)
- **THEN** those rows appear after all rows for fields in the fixed order list, in their originally inferred order

---

### Requirement: DESCRIBE TABLE is case-insensitive
The parser SHALL accept `describe table pods`, `DESCRIBE TABLE pods`, and `Describe Table Pods` as equivalent.

#### Scenario: Lowercase variant accepted
- **WHEN** the user runs `kubectl-sql "describe table pods"`
- **THEN** the command exits 0 and returns the column table
