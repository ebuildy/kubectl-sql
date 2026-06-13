## MODIFIED Requirements

### Requirement: No flattened underscore alias columns
There SHALL be no synthetic `metadata_labels`, `metadata_labels_app` style alias columns. Nested field access is expressed via the `->` operator only; no query-string rewriting step exists to support a dot-notation form.

#### Scenario: Arrow notation accesses nested fields directly
- **WHEN** the user runs `SELECT metadata->labels->app FROM pods`
- **THEN** the query is parsed and executed as written, with no rewrite step, and returns `nginx`
