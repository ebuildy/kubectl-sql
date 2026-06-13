## ADDED Requirements

### Requirement: Local JSON Lines files are queryable via FROM
A `FROM` reference whose table name ends in `.json`, `.jsonl`, or `.ndjson` SHALL be
resolved as a local JSON Lines file (one JSON object per line), read relative to the
current working directory (or as an absolute/relative path). The engine SHALL infer
a column schema by sampling the file and SHALL expose the inferred top-level JSON
keys as columns, supporting `SELECT` column lists, `*`, `WHERE`, `ORDER BY`, `LIMIT`,
and the `table`, `json`, and `csv` output formats identically to a Kubernetes-backed
table.

#### Scenario: SELECT * reads a JSON Lines file
- **WHEN** the user runs `kubectl-sql "SELECT * FROM notes.json"` where `notes.json`
  contains one JSON object per line, e.g. `{"pod":"nginx-1","note":"ok"}`
- **THEN** the output contains one row per line of `notes.json`, with one column per
  JSON key, each rendered with a table-qualifier prefix (e.g. `notes.pod`,
  `notes.note`) consistent with how Kubernetes-backed tables render columns as
  `<table>.<field>` (e.g. `pods.name`)

#### Scenario: jsonl and ndjson extensions are recognized
- **WHEN** the user runs `kubectl-sql "SELECT * FROM notes.jsonl"` or
  `kubectl-sql "SELECT * FROM notes.ndjson"` against a JSON Lines file with that
  extension
- **THEN** the file is read the same way as a `.json` file — same schema inference
  and row data — though the auto-derived column-qualifier prefix for `.jsonl` and
  `.ndjson` references is the literal extension (`jsonl`/`ndjson`) rather than the
  file's basename, a quirk of octosql's SQL grammar (`json` is a recognized
  non-reserved keyword usable as a table-name suffix; `jsonl`/`ndjson` are not). An
  explicit `AS <alias>` (e.g. `FROM notes.jsonl AS notes`) gives predictable,
  consistent column prefixes across all three extensions

#### Scenario: Column selection and WHERE filter JSON file fields
- **WHEN** the user runs
  `kubectl-sql "SELECT pod, note FROM notes.json WHERE pod = 'nginx-1'"`
- **THEN** only the `pod` and `note` columns are returned, restricted to rows whose
  `pod` field equals `nginx-1`

#### Scenario: ORDER BY, LIMIT, and output formats work
- **WHEN** the user runs
  `kubectl-sql -o json "SELECT * FROM notes.json ORDER BY pod LIMIT 1"`
- **THEN** the result is sorted by `pod`, capped at one row, and rendered as JSON —
  the same `ORDER BY`/`LIMIT`/output-format handling used for Kubernetes-backed
  tables

### Requirement: Multiple JSON file tables can be combined via JOIN
The engine SHALL resolve each `FROM`/`JOIN` table reference independently, so a
single query MAY `JOIN` two (or more) local JSON Lines files.

#### Scenario: JOIN between two JSON Lines files
- **WHEN** the user runs
  `kubectl-sql "SELECT n.pod, n.note, s.status FROM notes.json n JOIN status.json s ON n.pod = s.pod"`
  where `notes.json` contains lines like `{"pod":"nginx-1","note":"ok"}` and
  `status.json` contains lines like `{"pod":"nginx-1","status":"Running"}`
- **THEN** the result joins rows from both files, matching `n.pod` to `s.pod`, with
  one output row per matching pair

> **Note:** `JOIN` between a `k8s.*`-routed table (e.g. `pods`) and any other table,
> including a `*.json` table, is out of scope for this capability — see `design.md`
> Non-Goals for the pre-existing `KubernetesDatabase` join-execution issue this
> depends on.

### Requirement: JSON Lines format is required; non-conforming files error clearly
Each line of a file referenced via `.json`, `.jsonl`, or `.ndjson` SHALL be a single
JSON object. A file that is not in this form (e.g. a single top-level JSON array, or
an object pretty-printed across multiple lines) SHALL cause the query to fail with
an error identifying the problem, rather than silently returning no rows or
misparsed data.

#### Scenario: A top-level JSON array file fails with a clear error
- **WHEN** the user runs `kubectl-sql "SELECT * FROM data.json"` where `data.json`
  contains a single pretty-printed JSON array (not one object per line)
- **THEN** the command exits non-zero with an error message indicating the line
  could not be parsed as a JSON object

### Requirement: File table names follow path-based identifier syntax
`FROM` table names for JSON file sources SHALL accept relative paths (e.g.
`fixtures/notes.json`), explicit relative paths (e.g. `./notes.json`), and absolute
paths (e.g. `/tmp/notes.json`), consistent with octosql's identifier-based path
syntax. Because the engine registers a Kubernetes database under the name `k8s`, a
file whose name (without extension) is exactly `k8s` (e.g. `k8s.json`) SHALL be
referenced with a leading `./` (e.g. `./k8s.json`) to avoid being interpreted as
`<k8s database>.json`.

#### Scenario: A path with a directory component resolves to the file
- **WHEN** the user runs `kubectl-sql "SELECT * FROM fixtures/notes.json"`
- **THEN** the file at `fixtures/notes.json` (relative to the current working
  directory) is read as a JSON Lines source

#### Scenario: A file named k8s.json requires a ./ prefix
- **WHEN** the user runs `kubectl-sql "SELECT * FROM k8s.json"` with a file named
  `k8s.json` in the current directory
- **THEN** the query fails because `k8s.json` is interpreted as resource `json` in
  the `k8s` database, not the local file
- **WHEN** the user instead runs `kubectl-sql "SELECT * FROM ./k8s.json"`
- **THEN** the local file `k8s.json` is read as a JSON Lines source
