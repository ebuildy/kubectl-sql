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

### Requirement: Pretty table cells render multi-line string values with real line breaks (JSON beautify format)

The renderer SHALL render embedded newlines as real line break characters when the active beautify
cell format is JSON (the default) and a pretty-printed (beautify) struct/list/tuple/map cell's JSON
contains a string value with one or more embedded newlines, instead of the literal `\n` escape
sequence. All other JSON escape sequences (`\"`, `\\`, `\t`, `\uXXXX`, etc.)
SHALL remain JSON-escaped and unchanged. This applies only to table output with beautify enabled
(`pretty=true`); `--output json`, `--output csv`, and `--disable-beauty` cells SHALL continue to
contain the literal `\n` escape sequence and remain valid, single-line-per-record JSON, unchanged
from before this change.

#### Scenario: ConfigMap data with a multi-line script renders with real line breaks
- **WHEN** `kubectl-sql "SELECT name, data FROM configmaps"` is run with table output and beautify
  enabled, and a `data` entry's value is a shell script containing `\n` characters
- **THEN** the `data` cell's pretty JSON shows that value's content across multiple real lines,
  with no literal `\n` characters, while `\"` and other escapes inside the value remain
  JSON-escaped

#### Scenario: --output json keeps newlines escaped
- **WHEN** `kubectl-sql --output json "SELECT data FROM configmaps"` is run on data with a
  multi-line string value
- **THEN** that value remains valid JSON with `\n` escape sequences, unchanged from before this
  change

#### Scenario: --output csv and --disable-beauty keep newlines escaped
- **WHEN** `kubectl-sql --output csv "SELECT data FROM configmaps"` or
  `kubectl-sql --disable-beauty "SELECT data FROM configmaps"` is run on data with a multi-line
  string value
- **THEN** that cell remains compact, single-line JSON with `\n` escape sequences

#### Scenario: Key coloring is computed before newline conversion
- **WHEN** beautify and color-keys are enabled and a cell contains a multi-line string value
- **THEN** ANSI key coloring is applied to the original single-line-per-key JSON before embedded
  `\n` sequences are converted to real line breaks, so coloring is unaffected by the embedded
  newlines

---

### Requirement: Beautify cell rendering format is selectable between YAML and JSON via an internal constant

The table renderer SHALL support two beautify cell formats for pretty-printed (`pretty=true`)
struct/list/tuple/map cells: YAML (default) and JSON (per the requirement above). The active
format SHALL be controlled by a single internal Go constant in
`internal/adapter/sql/octosql/render.go`, not by a CLI flag. In YAML format, the cell SHALL contain
the same data as the JSON form (field names resolved from the schema, map columns decoded to
objects, list/tuple elements decoded), marshaled as YAML; string values containing newlines SHALL
render using YAML literal block scalar style (`|`) wherever the YAML library's formatting rules
permit, producing real line breaks without `\n` escapes. `--output json`, `--output csv`, and
`--disable-beauty` are unaffected by this constant and always use JSON.

#### Scenario: YAML beautify format renders a struct cell as YAML
- **WHEN** `kubectl-sql "SELECT name, status FROM pods"` is run with table output and beautify
  enabled (the default beautify format, YAML)
- **THEN** the `status` cell contains YAML (e.g. `phase: Running`) instead of indented JSON

#### Scenario: YAML beautify format renders multi-line strings as block scalars
- **WHEN** the active beautify format is YAML and a cell value contains a string with embedded
  newlines (e.g. a ConfigMap `data.teardown` script)
- **THEN** that value renders using YAML literal block scalar style (`|`) with real line breaks,
  not a quoted scalar with `\n` escapes

#### Scenario: YAML is the default
- **WHEN** the internal beautify format constant is left at its default value
- **THEN** beautify cell rendering is YAML

#### Scenario: JSON remains available via the internal constant
- **WHEN** the internal beautify format constant is set to `beautifyFormatJSON`
- **THEN** beautify cell rendering is pretty-printed JSON, with real line breaks for multi-line
  string values and full-depth key coloring, per the requirement above

---

### Requirement: YAML beautify cells color top-level keys when ColorKeys is enabled

The renderer SHALL wrap the top-level (root, column-0) mapping keys of a pretty-printed struct/map
cell in ANSI cyan when the active beautify cell format is YAML and `--color-keys` (`ColorKeys`) is
enabled, matching the coloring style applied to JSON keys (`ColorizeJSONKeys`). Keys of nested
maps, keys of sequence-item maps (e.g. `- name: c1`), scalar values, and the content of literal
block scalars SHALL NOT be colorized — colorization is restricted to column-0 mapping keys so that
the indented content of a literal block scalar (always indented relative to its key) can never be
mistaken for a key. With `ColorKeys` disabled (or `--no-color`), YAML cells render with no ANSI
codes, unchanged from before.

#### Scenario: Top-level struct keys are colored
- **WHEN** `--color-keys` is enabled, the active beautify format is YAML, and
  `kubectl-sql "SELECT name, status FROM pods"` is run with table output and beautify enabled
- **THEN** the `status` cell's root-level key (e.g. `phase`) is wrapped in ANSI cyan, while its
  value (`Running`) and any nested keys (e.g. `ready` under `conditions`) are not colored

#### Scenario: Block scalar content is never colorized as a key
- **WHEN** `--color-keys` is enabled, the active beautify format is YAML, and a cell's root-level
  key's value is a multi-line string rendered as a literal block scalar (`|`)
- **THEN** only the root-level key itself is colored; lines of the block scalar's content, even if
  they resemble `key: value`, are never colorized

#### Scenario: Sequence-rooted cells are not colorized
- **WHEN** `--color-keys` is enabled, the active beautify format is YAML, and a cell's value is a
  list (rendered as a YAML sequence at the document root, e.g. `- name: c1`)
- **THEN** the sequence-item keys are not colorized (no column-0 mapping key exists in the cell)

#### Scenario: Color keys disabled leaves YAML cells uncolored
- **WHEN** `--color-keys` is disabled (or `--no-color` is set) and the active beautify format is
  YAML
- **THEN** YAML cells render with no ANSI escape codes, unchanged from before this requirement

---

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

---

### Requirement: Pretty struct rendering can be disabled with --disable-beauty

The CLI SHALL provide a `--disable-beauty` flag (default `false`). When set, struct-typed cells in
table output SHALL render as compact single-line JSON with no ANSI coloring, regardless of the
active beautify format (YAML or JSON). The flag SHALL have no effect on `--output json` or on
scalar cell rendering.

#### Scenario: Flag switches table cells to compact JSON
- **WHEN** `kubectl-sql --disable-beauty "SELECT name, status FROM pods"` is run with table output
- **THEN** the `status` cell contains compact single-line JSON with field names as keys and no
  indentation or color, even if the active beautify format is YAML

#### Scenario: Flag default keeps pretty rendering
- **WHEN** the flag is not passed
- **THEN** struct cells render using the active beautify format (YAML by default, or JSON if the
  internal constant is set to `beautifyFormatJSON`)

---

### Requirement: JSON keys are colored in pretty struct cells on TTY

The renderer SHALL colorize JSON object keys at every nesting depth (and only keys — never values,
braces, or punctuation) with an ANSI color (`ColorizeJSONKeys`) when rendering pretty-printed
struct/list/tuple/map cells in table output and the active beautify format is JSON. Coloring SHALL be
applied only when stdout is a TTY, `--no-color` is not set, and `--disable-beauty` is not set. CSV
and `--output json` output SHALL never contain ANSI escape codes. When the active beautify format
is YAML (the default), key coloring is scoped to top-level keys only — see "YAML beautify cells
color top-level keys when ColorKeys is enabled" above.

#### Scenario: Keys colored on interactive terminal
- **WHEN** `kubectl-sql "SELECT status FROM pods"` runs with stdout attached to a TTY, colors
  enabled, and the active beautify format is JSON
- **THEN** JSON keys in struct cells, at every nesting depth, are wrapped in ANSI color codes and
  values are uncolored

#### Scenario: No color when piped
- **WHEN** the same query runs with stdout redirected to a file or pipe
- **THEN** the output contains no ANSI escape codes

#### Scenario: --no-color disables key coloring
- **WHEN** `kubectl-sql --no-color "SELECT status FROM pods"` runs on a TTY
- **THEN** struct cells are pretty-printed (in the active beautify format) with no ANSI escape
  codes

#### Scenario: Machine formats are never colored
- **WHEN** `kubectl-sql --output csv` or `--output json` is run on a TTY
- **THEN** the output contains no ANSI escape codes
