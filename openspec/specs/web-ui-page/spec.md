# Spec: Web UI Page

## Purpose

Defines the embedded single-page UI: a self-contained, framework-free page with a syntax-highlighted
SQL editor, completion-driven autocomplete, and a results section that renders rows as an HTML table
(with colored-YAML object cells, resizable columns, and URL/history integration).

---

## Requirements

### Requirement: Self-contained single-page UI

The server SHALL serve a single HTML page at `GET /` that is self-contained: all CSS and
JavaScript needed to run the UI SHALL be embedded in the binary (via `go:embed`) and served by
the plugin, with no dependency on a CDN, external network access, or a build step. The UI SHALL
NOT use a heavyweight front-end framework (no React/Vue/Angular/bundler) — only vanilla
JavaScript and a small amount of CSS.

#### Scenario: Page loads offline

- **WHEN** the browser requests `GET /` while offline (no internet)
- **THEN** the full UI loads and is interactive using only assets served by the plugin

#### Scenario: Assets are embedded

- **WHEN** the plugin binary is run from a directory containing no UI source files
- **THEN** `kubectl sql --ui` still serves the complete UI

### Requirement: Two-section layout

The page SHALL present two sections: (1) a SQL editor where the user types a query, and (2) a
results section that displays query output. A control SHALL exist to submit the current query
(button and/or keyboard shortcut).

#### Scenario: Layout is present

- **WHEN** the page loads
- **THEN** a SQL editor input area is visible
- **AND** a results area is visible
- **AND** a way to run the query is available

### Requirement: SQL editor with syntax highlighting

The SQL editor SHALL highlight SQL syntax (at minimum: keywords, strings, and the `->` field
accessor) as the user types, using lightweight client-side logic without a heavy editor library.

#### Scenario: Keywords are highlighted

- **WHEN** the user types `SELECT name FROM pods WHERE status->phase = 'Running'`
- **THEN** SQL keywords are visually distinguished from identifiers and string literals

### Requirement: Autocomplete in the editor

The SQL editor SHALL offer autocomplete suggestions driven by the completion API, showing
candidate keywords, table (resource) names, and column names relevant to the current cursor
position. Selecting a suggestion SHALL insert it into the editor.

#### Scenario: Completion suggestions appear

- **WHEN** the user has typed a partial token and triggers completion
- **THEN** the UI requests candidates from the completion endpoint for the current line and cursor
- **AND** displays the returned candidates
- **AND** inserting a candidate updates the editor text

#### Scenario: No candidates

- **WHEN** the completion endpoint returns no candidates for the current position
- **THEN** the UI shows no suggestion popup and does not error

### Requirement: Results rendered as an HTML table

When a query succeeds, the results section SHALL render the returned rows as an HTML table with a
header row of column names and one table row per result row. When a query fails, the results
section SHALL display the error message (and the typo-correction suggestion, if provided) instead
of a table.

#### Scenario: Successful query renders a table

- **WHEN** a submitted query returns columns and rows
- **THEN** the results section shows an HTML table whose header is the column names
- **AND** each result row appears as a table row

#### Scenario: Empty result set

- **WHEN** a submitted query returns zero rows
- **THEN** the results section indicates that no rows matched (e.g. an empty-state message), not a broken table

#### Scenario: Query error is shown

- **WHEN** a submitted query fails
- **THEN** the results section displays the error message returned by the API
- **AND** if the API returned a suggested corrected query, it is offered to the user

### Requirement: Object cells rendered as colored YAML

Composite result cells (objects or arrays, e.g. struct or map columns) SHALL render as YAML rather
than JSON, with syntax coloring (at minimum: distinct colors for keys and string/number/boolean/null
scalars), mirroring how the CLI table renderer presents struct cells. Scalar cells SHALL render as
plain text.

#### Scenario: Object cell renders as colored YAML

- **WHEN** a result row contains an object-valued column (e.g. `metadata->labels`)
- **THEN** that cell is displayed as YAML (key/value lines), not a JSON blob
- **AND** the YAML keys are visually distinguished from scalar values by color

#### Scenario: Scalar cell renders as plain text

- **WHEN** a result cell holds a scalar (string, number, boolean, or null)
- **THEN** it is rendered as plain text without YAML structure

### Requirement: Query reflected in URL and browser history

The submitted query SHALL be reflected in the page URL via a `sql` query-string parameter so the
state is bookmarkable and shareable. Submitting a new query SHALL push a new browser history entry,
and the browser Back/Forward buttons SHALL restore the corresponding query into the editor and
re-run it. On load, a query present in the URL SHALL pre-fill the editor and run immediately.

#### Scenario: Submitting a query updates the URL

- **WHEN** the user runs a query
- **THEN** the page URL is updated to include the query in a `sql` parameter
- **AND** a new history entry is added only when the query differs from the current URL

#### Scenario: Back restores the previous query

- **WHEN** the user has run two different queries and presses the browser Back button
- **THEN** the editor is restored to the previous query and its results are shown again

#### Scenario: URL query pre-fills the editor on load

- **WHEN** the page is opened with a `sql` query-string parameter
- **THEN** the editor is pre-filled with that query and the query runs immediately

### Requirement: Resizable result columns

The result table columns SHALL be resizable: the user can drag the right edge of any column header
to change that column's width, with a sensible minimum width.

#### Scenario: Dragging a column header resizes the column

- **WHEN** the user drags the resize handle on a column header
- **THEN** that column's width changes to follow the pointer
- **AND** the column does not shrink below a minimum width
