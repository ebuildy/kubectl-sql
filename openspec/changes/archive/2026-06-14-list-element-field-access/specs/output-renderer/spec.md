## MODIFIED Requirements

### Requirement: Struct values render as JSON in table and CSV output

When a result cell holds a struct-typed value, the renderer SHALL resolve its field names from the
schema type, decode list/tuple values into arrays, and decode map columns (carried as flat
key/value lists) into objects — matching what `--output json` produces for the same cell. When a
cell holds a **list whose element type is a struct** (`List<Struct>`), the renderer SHALL decode
each element into an object using the element struct type for field names (rather than treating the
element as an opaque JSON-encoded string), so list-of-object columns render as arrays of named-key
objects. CSV output SHALL always render the cell as compact single-line JSON inside a properly
quoted CSV field. Table output with beautify enabled (`pretty=true`, `--disable-beauty` not set)
SHALL render the cell using the active beautify cell format (an internal Go constant in
`internal/adapter/sql/octosql/render.go`, defaulting to YAML, per "Beautify cell rendering format
is selectable between YAML and JSON via an internal constant"): YAML format renders indented YAML
(multi-line string values as literal block scalars, `|`); JSON format renders 2-space-indented
JSON with embedded `\n` escapes converted to real line breaks. `List<Struct>` cells SHALL flow
through this same beautify path — i.e. render as pretty YAML by default. Scalar values SHALL render
exactly as before. If conversion fails, the renderer SHALL fall back to the octosql string form
rather than returning an error.

#### Scenario: Struct cell in table output is pretty YAML by default
- **WHEN** `kubectl-sql "SELECT name, status FROM pods"` is run with table output and beautify
  enabled, and `status` is a struct column
- **THEN** the `status` cell contains indented YAML with field names as keys (e.g.
  `phase: Running`), not octosql's positional `{ v1, v2 }` form or JSON

#### Scenario: Struct cell in table output renders as pretty JSON via the internal constant
- **WHEN** the internal beautify format constant is set to `beautifyFormatJSON` and the same query
  is run with table output and beautify enabled
- **THEN** the `status` cell contains indented JSON with field names as keys (e.g.
  `{"phase": "Running", ...}`), with any embedded `\n` escapes in string values rendered as real
  line breaks

#### Scenario: Struct cell in CSV output is compact JSON
- **WHEN** `kubectl-sql --output csv "SELECT name, status FROM pods"` is run and `status` is a
  struct column
- **THEN** the `status` field contains single-line compact JSON, the record remains one CSV line,
  and the output parses with a standard CSV reader

#### Scenario: Nested struct fields are resolved recursively
- **WHEN** a struct column contains a nested struct field
- **THEN** the nested value is rendered as a nested object (YAML or JSON, per the active beautify
  format) with its own field names

#### Scenario: List and tuple cells render as arrays
- **WHEN** a query selects a list column (e.g. `spec.containers`) or produces a tuple value with
  table output
- **THEN** the cell contains a pretty-printed array (a YAML sequence by default, or a JSON array
  if the internal constant is set to `beautifyFormatJSON`), with JSON-string elements decoded to
  objects

#### Scenario: List-of-struct cells render as arrays of named-key objects
- **WHEN** a query selects a list column whose element type is a struct (e.g.
  `SELECT spec->containers FROM pods`) with table output and beautify enabled
- **THEN** the cell contains a pretty YAML sequence (default) of mappings whose keys are the
  element struct field names (e.g. `- name: nginx` / `  image: nginx`), not a sequence of escaped
  JSON strings, and `--output json` renders the same column as an array of objects

#### Scenario: Map cells render as objects
- **WHEN** a query selects a map column (e.g. `metadata.labels`) with table output
- **THEN** the cell contains a pretty-printed object (a YAML mapping by default, or a JSON object
  if the internal constant is set to `beautifyFormatJSON`) keyed by the map keys, not the flat
  alternating key/value list

#### Scenario: Scalar cells are unchanged
- **WHEN** a query selects only scalar columns (string, int, float, boolean, time, null)
- **THEN** table and CSV cell rendering is byte-identical to the previous behavior

#### Scenario: JSON output format is unaffected
- **WHEN** `kubectl-sql --output json "SELECT status FROM pods"` is run
- **THEN** the output is identical to the behavior before this change
