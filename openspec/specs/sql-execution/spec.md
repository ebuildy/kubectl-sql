# Spec: SQL Execution

## Purpose

Defines the end-to-end SQL query execution contract: how queries are accepted from the CLI, how SQL constructs (SELECT, WHERE, LIMIT) are applied against Kubernetes data, and how flags are forwarded to the underlying client. These requirements govern the user-facing query behavior of `kubectl-sql`.

---

## Requirements

### Requirement: SQL query is accepted as a positional argument
The root command SHALL accept a single positional SQL string as its first argument and execute it against the Kubernetes cluster. If the query is `SHOW TABLES` (case-insensitive), it SHALL be handled before the octosql pipeline and return a table of all queryable Kubernetes resource types. Results SHALL be rendered by `internal/output.Render` so the process always exits cleanly regardless of terminal environment. When no positional argument is provided, the command SHALL open the REPL: interactively on a TTY, or in line-by-line batch mode when stdin is piped.

#### Scenario: Query executes and prints a table
- **WHEN** the user runs `kubectl-sql "SELECT name, namespace FROM pods"`
- **THEN** the command connects to the cluster, fetches pods, and prints a table with columns `name` and `namespace` to stdout, then exits 0

#### Scenario: Missing argument opens REPL on TTY
- **WHEN** the user runs `kubectl-sql` with no arguments and stdin is a TTY
- **THEN** the command opens the interactive REPL prompt instead of showing help

#### Scenario: Missing argument with piped stdin runs batch mode
- **WHEN** the user runs `kubectl-sql` with no arguments and stdin is piped (not a TTY)
- **THEN** the command reads queries line-by-line from stdin, executes each, and exits 0 at EOF

#### Scenario: Invalid SQL prints an error
- **WHEN** the user runs `kubectl-sql "NOT VALID SQL"`
- **THEN** the command prints an error message to stderr and exits 1

#### Scenario: SHOW TABLES is handled before SQL parsing
- **WHEN** the user runs `kubectl-sql "SHOW TABLES"`
- **THEN** the command returns a table of resource types without invoking the octosql pipeline, and exits 0

#### Scenario: Process exits without hanging in any terminal environment
- **WHEN** `kubectl-sql` is run from a VS Code terminal, tmux, SSH session, or any environment without a controlling TTY
- **THEN** the process prints results and exits 0 without hanging

---

### Requirement: SELECT wildcard returns all fields
The query `SELECT * FROM <resource>` SHALL return all top-level fields of the resource as columns.

#### Scenario: Wildcard query on pods
- **WHEN** the user runs `kubectl-sql "SELECT * FROM pods"`
- **THEN** the table includes at least the columns present in the resource's unstructured JSON

---

### Requirement: WHERE clause filters rows
The WHERE clause SHALL filter the result set so only matching rows are returned.

#### Scenario: Phase filter
- **WHEN** the user runs `kubectl-sql "SELECT name FROM pods WHERE status.phase = 'Running'"`
- **THEN** only pods with `status.phase == "Running"` appear in the output

---

### Requirement: LIMIT clause restricts row count
The LIMIT clause SHALL cap the number of rows returned in the output.

#### Scenario: LIMIT 5
- **WHEN** the user runs `kubectl-sql "SELECT name FROM pods LIMIT 5"`
- **THEN** at most 5 rows are printed

---

### Requirement: Kubeconfig flags are forwarded to the client
The `--context`, `--namespace`, `--kubeconfig`, `--page-size`, and `--timeout` flags SHALL be applied when executing the query.

#### Scenario: Namespace flag restricts results
- **WHEN** the user runs `kubectl-sql -n kube-system "SELECT name FROM pods"`
- **THEN** only pods in the `kube-system` namespace appear in the output

---

### Requirement: --watch flag re-executes the query on a polling interval
When `--watch` / `-w` is set, the command SHALL run the full query in a polling loop, re-executing every 5 seconds and reprinting the result table until interrupted (SIGINT or `--timeout`).

#### Scenario: --watch re-executes the query on every tick
- **WHEN** the user runs `kubectl-sql --watch "SELECT name FROM pods"`
- **THEN** the full query pipeline runs, the table is printed, and after 5 seconds the table is cleared and reprinted with fresh data

#### Scenario: --watch respects all SQL clauses
- **WHEN** the user runs `kubectl-sql --watch "SELECT name FROM pods ORDER BY name LIMIT 10"`
- **THEN** the query runs normally with ORDER BY and LIMIT applied on every tick

#### Scenario: --watch exits cleanly on SIGINT
- **WHEN** the user presses Ctrl-C while watching
- **THEN** the command exits 0

---

### Requirement: List element fields are accessible by index and field path
The query engine SHALL allow accessing a single element of a list-typed column that carries a known element object schema (its element is a struct) by integer index, and then accessing that element's fields with the `->` operator, i.e. `list[index]->field` (and deeper, e.g. `list[index]->sub->field`). Indexing SHALL return the element struct, or NULL when the index is out of range, and field access on a possibly-NULL element SHALL yield NULL rather than erroring. Element struct values SHALL materialize from the raw resource object so the projected values match the underlying data. List columns without a known element schema SHALL keep their existing scalar/JSON-string element behavior (`array_get` / indexing returns a JSON-encoded element).

#### Scenario: Selecting a nested list element field returns the value
- **WHEN** the user runs `kubectl-sql "SELECT name, spec->containers[0]->name FROM pods LIMIT 2"`
- **THEN** the query type-checks and executes, and the second column shows each pod's first
  container name (not a JSON blob or an error), then exits 0

#### Scenario: Out-of-range index yields NULL
- **WHEN** the user selects `spec->containers[99]->name` for a pod with fewer than 100 containers
- **THEN** the cell resolves to NULL and the query exits 0 without error

#### Scenario: List element field usable in WHERE
- **WHEN** the user runs `kubectl-sql "SELECT name FROM pods WHERE spec->containers[0]->image = 'nginx'"`
- **THEN** the query executes and returns pods whose first container image is `nginx`

#### Scenario: Scalar-element list access is unchanged
- **WHEN** the user indexes a list column that has no known element object schema (e.g. a `[]string`)
- **THEN** indexing returns the JSON-encoded element as before, with no regression
