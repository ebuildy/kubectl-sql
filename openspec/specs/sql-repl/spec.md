# Spec: SQL REPL

## Purpose

Defines the interactive Read-Eval-Print-Loop entered when `kubectl-sql` is run with no positional query: the prompt loop, query execution and error handling, exit and help commands, flag inheritance, session history, non-TTY batch fallback, and Tab autocomplete for SQL keywords, table names, and column names.

---

## Requirements

### Requirement: REPL mode opens when no query is provided
When the user runs `kubectl-sql` with no positional argument (and no `--repl` flag), the command SHALL open an interactive REPL instead of showing help. The REPL SHALL display a `sql> ` prompt and wait for input.

#### Scenario: Bare invocation opens REPL
- **WHEN** the user runs `kubectl-sql` with no arguments in a TTY
- **THEN** the command prints `sql> ` prompt and waits for SQL input without exiting

#### Scenario: --repl flag opens REPL regardless of arguments
- **WHEN** the user runs `kubectl-sql --repl`
- **THEN** the command opens the REPL prompt

### Requirement: REPL executes SQL and prints results
Each line of input (terminated by pressing Enter) SHALL be treated as a SQL query and executed. A trailing `;` SHALL be stripped before execution. Results SHALL be printed in the configured output format, then the prompt SHALL reappear.

#### Scenario: Valid query executes and returns to prompt
- **WHEN** the user types `SELECT name FROM pods LIMIT 3` and presses Enter
- **THEN** the results are printed and `sql> ` reappears

#### Scenario: Invalid query prints error and returns to prompt
- **WHEN** the user types `NOT VALID SQL` and presses Enter
- **THEN** an error is printed to stderr and `sql> ` reappears (REPL does not exit)

#### Scenario: Empty input is ignored
- **WHEN** the user presses Enter on an empty line
- **THEN** a new `sql> ` prompt is shown without executing anything

#### Scenario: Trailing semicolon is stripped
- **WHEN** the user types `SELECT name FROM pods;`
- **THEN** the query executed is `SELECT name FROM pods` (no trailing semicolon)

### Requirement: REPL exits cleanly on quit commands
The commands `\q`, `quit`, `exit` (case-insensitive) and Ctrl-C (SIGINT) SHALL exit the REPL with exit code 0.

#### Scenario: \q exits the REPL
- **WHEN** the user types `\q` and presses Enter
- **THEN** the REPL exits with code 0

#### Scenario: exit keyword exits the REPL
- **WHEN** the user types `exit` and presses Enter
- **THEN** the REPL exits with code 0

#### Scenario: Ctrl-C exits the REPL
- **WHEN** the user presses Ctrl-C at the idle prompt
- **THEN** the REPL exits with code 0

#### Scenario: Ctrl-C during a running query returns to the prompt
- **WHEN** the user presses Ctrl-C while a query is executing
- **THEN** the running query is cancelled, `^C` is printed, and the prompt reappears (REPL does not exit)

### Requirement: REPL provides a \help command
The `\help` command (and `?`) SHALL print a short summary of available REPL commands to stdout.

#### Scenario: \help prints command list
- **WHEN** the user types `\help`
- **THEN** the REPL prints at least: `\q` / `quit` / `exit` (exit REPL), `\help` / `?` (show help)

### Requirement: REPL inherits all CLI flags
All flags passed to `kubectl-sql` before entering the REPL SHALL apply to every query executed inside the REPL: `--output`, `--namespace`, `--context`, `--kubeconfig`, `--page-size`, `--timeout`, `--no-color`.

#### Scenario: --namespace flag scopes all REPL queries
- **WHEN** the user runs `kubectl-sql -n kube-system` (enters REPL) and queries `SELECT name FROM pods`
- **THEN** only pods in `kube-system` are returned

#### Scenario: --output flag formats all REPL output
- **WHEN** the user runs `kubectl-sql --output json` (enters REPL) and executes a query
- **THEN** results are printed as JSON

### Requirement: REPL maintains input history within a session
The REPL SHALL keep an in-memory history of entered queries for the current session. The up-arrow key SHALL navigate to previous entries.

#### Scenario: Up-arrow recalls previous query
- **WHEN** the user has executed at least one query and presses the up-arrow key
- **THEN** the previous query is shown at the prompt for editing

### Requirement: Non-TTY falls back to batch mode
When stdin is not a TTY (piped input), the command SHALL read queries line-by-line from stdin and execute each one, without printing a prompt, then exit 0 when stdin is exhausted. A per-query error SHALL be printed to stderr but SHALL NOT abort the batch.

#### Scenario: Piped queries execute in batch
- **WHEN** the user runs `echo "SELECT name FROM pods LIMIT 1" | kubectl-sql`
- **THEN** the query executes, results are printed, and the process exits 0

#### Scenario: Batch continues after a failing query
- **WHEN** a piped batch contains an invalid query followed by a valid one
- **THEN** the error for the invalid query is printed to stderr and the valid query still executes

### Requirement: Tab completes SQL keywords
In the interactive prompt, pressing Tab SHALL offer completions for SQL keywords. The keyword set SHALL include the statement starters kubectl-sql supports (`SELECT`, `SHOW`, `DESCRIBE`, `WITH`), the clauses/operators it accepts (at minimum: `FROM`, `WHERE`, `ORDER BY`, `GROUP BY`, `HAVING`, `LIMIT`, `OFFSET`, `DISTINCT`, `AS`, `AND`, `OR`, `NOT`, `IN`, `IS NULL`, `LIKE`, `BETWEEN`, `UNION`), and the join forms (`JOIN`, `INNER JOIN`, `LEFT JOIN`, `RIGHT JOIN`, `FULL JOIN`, `CROSS JOIN`, `ON`, `USING`). Because kubectl-sql is read-only, write statements (`UPDATE`, `DELETE`, `INSERT`) SHALL NOT be offered. Completion SHALL preserve the case of the partial word the user typed; when no word is typed, only statement starters SHALL be offered, in uppercase. Suggestions SHALL be ordered alphabetically (case-insensitive) and capped at 50 entries.

#### Scenario: Keyword prefix completes
- **WHEN** the user types `sel` and presses Tab
- **THEN** the completer offers `select` (case preserved from the typed prefix)

#### Scenario: Uppercase keyword prefix completes uppercase
- **WHEN** the user types `SEL` and presses Tab
- **THEN** the completer offers `SELECT`

#### Scenario: Join keywords complete
- **WHEN** the user types `inner` and presses Tab
- **THEN** the completer offers `inner join` (and uppercase `INNER` offers `INNER JOIN`)

#### Scenario: Statement starters complete at the start of the line
- **WHEN** the user types `sh` at the start of the line and presses Tab
- **THEN** the completer offers `show`

#### Scenario: Tab on an empty line offers statement starters
- **WHEN** the user presses Tab on an empty line
- **THEN** the completer offers the statement starters (`SELECT`, `SHOW`, `DESCRIBE`, `WITH`) in uppercase

#### Scenario: Write statements are not offered
- **WHEN** the user types `up` or `del` and presses Tab
- **THEN** the completer does not offer `UPDATE` or `DELETE` (kubectl-sql is read-only)

### Requirement: Tab completes table names after FROM
When the cursor is in a position where a table name is expected — immediately after a `FROM`/`JOIN` keyword, or after `TABLE` in a `DESCRIBE TABLE` statement — pressing Tab SHALL offer completions drawn from the set of queryable Kubernetes resources (the same set returned by `SHOW TABLES`), matched against the partial word typed.

#### Scenario: Table name completes after FROM
- **WHEN** the user types `SELECT name FROM po` and presses Tab
- **THEN** the completer offers resource names beginning with `po` (e.g. `pods`, `podtemplates`)

#### Scenario: Table name completes after DESCRIBE TABLE
- **WHEN** the user types `DESCRIBE TABLE po` and presses Tab
- **THEN** the completer offers resource names beginning with `po` (e.g. `pods`, `podtemplates`)

#### Scenario: Table completion source matches SHOW TABLES
- **WHEN** the completer lists table candidates
- **THEN** the candidate set is the same set of resource names produced by `SHOW TABLES`

### Requirement: Tab completes column names for the FROM table
When a query contains a resolvable `FROM <table>` and the cursor is positioned where a column reference is expected (e.g. after `SELECT` or `WHERE`), pressing Tab SHALL offer column-name completions inferred from that table's schema. The table's schema SHALL be prefetched and cached for the session when the table first appears in a completed `FROM` clause, so subsequent column completions do not block on a fresh inference.

#### Scenario: Column name completes from FROM table schema
- **WHEN** the user types `SELECT sta` against a query whose `FROM` clause is `pods` and presses Tab
- **THEN** the completer offers column names beginning with `sta` from the pods schema (e.g. `status`)

#### Scenario: Schema is cached after first use
- **WHEN** the pods schema has already been inferred for completion once in the session
- **THEN** a subsequent column completion for `pods` uses the cached schema and does not re-infer

#### Scenario: Unknown table yields no column completions
- **WHEN** the user requests column completion but the `FROM` table cannot be resolved
- **THEN** the completer returns no column candidates and does not error
