# Spec: Output Renderer

## Purpose

Defines the behavior of `internal/output.Render` — the direct result renderer that replaces octosql's `OutputPrinter`. It drives the octosql execution node, collects records, and writes output with no TTY dependency.

---

## Requirements

### Requirement: Output renderer collects and renders octosql records without TTY dependency
The `internal/output` package SHALL provide a `Render` function that drives an octosql `execution.Node`, collects all records, applies ORDER BY and LIMIT, and writes results to an `io.Writer` with no dependency on terminal state or `/dev/tty`.

#### Scenario: Table output renders to stdout
- **WHEN** `Render` is called with format `table` and a node that produces rows
- **THEN** a formatted table is written to the writer and the function returns nil

#### Scenario: Process exits cleanly after render
- **WHEN** `kubectl-sql "SELECT name FROM pods"` is run in any shell environment (TTY or not)
- **THEN** the process prints results and exits 0 without hanging

#### Scenario: COUNT(*) exits cleanly
- **WHEN** `kubectl-sql "SELECT COUNT(*) FROM pods"` is run
- **THEN** the process prints the count and exits 0 without hanging

---

### Requirement: Output format is controlled by --output flag
The renderer SHALL support `table` (default), `json`, and `csv` output formats selected via `--output/-o`.

#### Scenario: Default format is table
- **WHEN** `kubectl-sql "SELECT name FROM pods"` is run without `--output`
- **THEN** output is a formatted ASCII table

#### Scenario: JSON format outputs JSON array
- **WHEN** `kubectl-sql --output json "SELECT name FROM pods"` is run
- **THEN** output is a JSON array of objects, one per row, with field names as keys

#### Scenario: CSV format outputs comma-separated values
- **WHEN** `kubectl-sql --output csv "SELECT name FROM pods"` is run
- **THEN** output is CSV with a header row followed by one row per result

---

### Requirement: Beautify cell rendering format is YAML

The table renderer SHALL render pretty-printed (`pretty=true`) struct/list/tuple/map cells as YAML.
YAML is the only beautify cell format; there is no JSON beautify format and no internal
format-selection constant. In YAML format, the cell SHALL contain field names resolved from the
schema, map columns decoded to objects, and list/tuple elements decoded, marshaled as YAML; string
values containing newlines SHALL render using YAML literal block scalar style (`|`) wherever the
YAML library's formatting rules permit, producing real line breaks without `\n` escapes. `--output
json` and `--output csv` are unaffected and always use JSON.

#### Scenario: Beautify renders a struct cell as YAML
- **WHEN** `kubectl-sql "SELECT name, status FROM pods"` is run with table output and beautify
  enabled
- **THEN** the `status` cell contains YAML (e.g. `phase: Running`) instead of indented JSON

#### Scenario: Beautify renders multi-line strings as block scalars
- **WHEN** a cell value contains a string with embedded newlines (e.g. a ConfigMap `data.teardown`
  script) and beautify is enabled
- **THEN** that value renders using YAML literal block scalar style (`|`) with real line breaks,
  not a quoted scalar with `\n` escapes

---

### Requirement: YAML beautify cells omit fields with null values

When rendering beautify (YAML) cells, the renderer SHALL omit any mapping field whose value is null
from the rendered YAML, at every nesting depth, so that null-valued fields do not appear as
`field: null` lines. A field SHALL be omitted when its value is the SQL/octosql `NULL`, a JSON
`null`, or otherwise resolves to a nil value; fields with empty-but-non-null values (e.g. an empty
string `""`, empty list `[]`, empty map `{}`, zero number, or `false`) SHALL still be rendered. If
omitting null fields leaves a nested mapping empty, that mapping SHALL render as an empty mapping
(`{}`) rather than being removed, preserving its key. This omission applies only to YAML beautify
output; `--output json` and `--output csv` SHALL continue to include null fields unchanged.

#### Scenario: Null-valued struct field is omitted from YAML cell
- **WHEN** `kubectl-sql "SELECT name, status FROM pods"` is run with table output and beautify
  enabled, and the `status` struct has a field whose value is null (e.g. `status->reason` is null)
- **THEN** the `status` cell's YAML does not contain a `reason: null` line, while non-null fields
  (e.g. `phase: Running`) are still rendered

#### Scenario: Nested null fields are omitted at every depth
- **WHEN** a struct column contains a nested mapping in which one or more fields are null
- **THEN** those null fields are absent from the rendered YAML at their nesting depth, while their
  non-null siblings remain

#### Scenario: Empty-but-non-null values are preserved
- **WHEN** a YAML beautify cell contains a field whose value is an empty string, empty list, empty
  map, zero, or false (but not null)
- **THEN** that field is still rendered in the YAML and is not omitted

#### Scenario: Null fields are kept in non-YAML output
- **WHEN** the same query is run with `--output json` or `--output csv`
- **THEN** null-valued fields are included in the output, unchanged from before this requirement

---

### Requirement: YAML beautify cells color top-level keys when ColorKeys is enabled

The renderer SHALL wrap the top-level (root, column-0) mapping keys of a pretty-printed struct/map
cell in ANSI cyan when `--color-keys` (`ColorKeys`) is enabled. Keys of nested maps, keys of
sequence-item maps (e.g. `- name: c1`), scalar values, and the content of literal block scalars
SHALL NOT be colorized — colorization is restricted to column-0 mapping keys so that the indented
content of a literal block scalar (always indented relative to its key) can never be mistaken for a
key. With `ColorKeys` disabled (or `--no-color`), YAML cells render with no ANSI codes.

#### Scenario: Top-level struct keys are colored
- **WHEN** `--color-keys` is enabled and `kubectl-sql "SELECT name, status FROM pods"` is run with
  table output and beautify enabled
- **THEN** the `status` cell's root-level key (e.g. `phase`) is wrapped in ANSI cyan, while its
  value (`Running`) and any nested keys (e.g. `ready` under `conditions`) are not colored

#### Scenario: Block scalar content is never colorized as a key
- **WHEN** `--color-keys` is enabled and a cell's root-level key's value is a multi-line string
  rendered as a literal block scalar (`|`)
- **THEN** only the root-level key itself is colored; lines of the block scalar's content, even if
  they resemble `key: value`, are never colorized

#### Scenario: Sequence-rooted cells are not colorized
- **WHEN** `--color-keys` is enabled and a cell's value is a list (rendered as a YAML sequence at
  the document root, e.g. `- name: c1`)
- **THEN** the sequence-item keys are not colorized (no column-0 mapping key exists in the cell)

#### Scenario: Color keys disabled leaves YAML cells uncolored
- **WHEN** `--color-keys` is disabled (or `--no-color` is set)
- **THEN** YAML cells render with no ANSI escape codes

---

### Requirement: Struct values render as YAML in table output and JSON in CSV output

When a result cell holds a struct-typed value, the renderer SHALL resolve its field names from the
schema type, decode list/tuple values into arrays, and decode map columns (carried as flat
key/value lists) into objects — matching what `--output json` produces for the same cell. When a
cell holds a **list whose element type is a struct** (`List<Struct>`), the renderer SHALL decode
each element into an object using the element struct type for field names (rather than treating the
element as an opaque JSON-encoded string), so list-of-object columns render as arrays of named-key
objects. CSV output SHALL always render the cell as compact single-line JSON inside a properly
quoted CSV field. Table output with beautify enabled (`pretty=true`, `--disable-beauty` not set)
SHALL render the cell as YAML (multi-line string values as literal block scalars, `|`).
`List<Struct>` cells SHALL flow through this same beautify path — i.e. render as pretty YAML.
Scalar values SHALL render exactly as before. If conversion fails, the renderer SHALL fall back to
the octosql string form rather than returning an error.

#### Scenario: Struct cell in table output is pretty YAML
- **WHEN** `kubectl-sql "SELECT name, status FROM pods"` is run with table output and beautify
  enabled, and `status` is a struct column
- **THEN** the `status` cell contains indented YAML with field names as keys (e.g.
  `phase: Running`), not octosql's positional `{ v1, v2 }` form or JSON

#### Scenario: Struct cell in CSV output is compact JSON
- **WHEN** `kubectl-sql --output csv "SELECT name, status FROM pods"` is run and `status` is a
  struct column
- **THEN** the `status` field contains single-line compact JSON, the record remains one CSV line,
  and the output parses with a standard CSV reader

#### Scenario: Nested struct fields are resolved recursively
- **WHEN** a struct column contains a nested struct field
- **THEN** the nested value is rendered as a nested YAML mapping with its own field names

#### Scenario: List and tuple cells render as arrays
- **WHEN** a query selects a list column (e.g. `spec.containers`) or produces a tuple value with
  table output
- **THEN** the cell contains a pretty-printed YAML sequence, with JSON-string elements decoded to
  objects

#### Scenario: List-of-struct cells render as arrays of named-key objects
- **WHEN** a query selects a list column whose element type is a struct (e.g.
  `SELECT spec->containers FROM pods`) with table output and beautify enabled
- **THEN** the cell contains a pretty YAML sequence of mappings whose keys are the element struct
  field names (e.g. `- name: nginx` / `  image: nginx`), not a sequence of escaped JSON strings,
  and `--output json` renders the same column as an array of objects

#### Scenario: Map cells render as objects
- **WHEN** a query selects a map column (e.g. `metadata.labels`) with table output
- **THEN** the cell contains a pretty-printed YAML mapping keyed by the map keys, not the flat
  alternating key/value list

#### Scenario: Scalar cells are unchanged
- **WHEN** a query selects only scalar columns (string, int, float, boolean, time, null)
- **THEN** table and CSV cell rendering is byte-identical to the previous behavior

#### Scenario: JSON output format is unaffected
- **WHEN** `kubectl-sql --output json "SELECT status FROM pods"` is run
- **THEN** the output is identical to the behavior before this change

---

### Requirement: Pretty struct rendering can be disabled with --disable-beauty

The CLI SHALL provide a `--disable-beauty` flag (default `false`). When set, struct-typed cells in
table output SHALL render as compact single-line JSON with no ANSI coloring. The flag SHALL have no
effect on `--output json` or on scalar cell rendering.

#### Scenario: Flag switches table cells to compact JSON
- **WHEN** `kubectl-sql --disable-beauty "SELECT name, status FROM pods"` is run with table output
- **THEN** the `status` cell contains compact single-line JSON with field names as keys and no
  indentation or color

#### Scenario: Flag default keeps pretty rendering
- **WHEN** the flag is not passed
- **THEN** struct cells render as pretty YAML
