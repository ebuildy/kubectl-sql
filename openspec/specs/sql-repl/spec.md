# Spec: SQL REPL

## Purpose

Defines the interactive Read-Eval-Print-Loop entered when `kubectl-sql` is run with no positional query: the prompt loop, query execution and error handling, the slash-command set (`/quit`, `/clear`, `/history-clear`, `/help`, `/version`, `/tables`) and slash dispatch rule, flag inheritance, session history, non-TTY batch fallback, and Tab autocomplete for SQL keywords, table names, column names, function names, and slash commands.

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
The command `/quit` and the bare-word commands `quit`, `exit` (case-insensitive) and Ctrl-C (SIGINT) SHALL exit the REPL with exit code 0. The legacy `\q` command SHALL NOT be recognized.

#### Scenario: /quit exits the REPL
- **WHEN** the user types `/quit` and presses Enter
- **THEN** the REPL exits with code 0

#### Scenario: exit keyword exits the REPL
- **WHEN** the user types `exit` and presses Enter
- **THEN** the REPL exits with code 0

#### Scenario: quit keyword exits the REPL
- **WHEN** the user types `quit` and presses Enter
- **THEN** the REPL exits with code 0

#### Scenario: Ctrl-C exits the REPL
- **WHEN** the user presses Ctrl-C at the idle prompt
- **THEN** the REPL exits with code 0

#### Scenario: Ctrl-C during a running query returns to the prompt
- **WHEN** the user presses Ctrl-C while a query is executing
- **THEN** the running query is cancelled, `^C` is printed, and the prompt reappears (REPL does not exit)

#### Scenario: Legacy backslash quit is no longer recognized
- **WHEN** the user types `\q` and presses Enter
- **THEN** the REPL does not exit; it prints an error and returns to the prompt

### Requirement: REPL provides a /help command
The `/help` command SHALL print a short summary of available REPL slash commands to stdout. The legacy `\help` and `?` commands SHALL NOT be recognized.

#### Scenario: /help prints command list
- **WHEN** the user types `/help`
- **THEN** the REPL prints at least: `/quit` (exit REPL), `/clear` (clear screen), `/history-clear` (clear history), `/help` (show help), `/version` (show version), `/tables` (list tables)

#### Scenario: Legacy backslash help is no longer recognized
- **WHEN** the user types `\help` or `?`
- **THEN** the REPL does not print the help summary; it prints an error and returns to the prompt

### Requirement: REPL treats slash-prefixed lines as commands
An input line whose first non-space character is `/` SHALL be interpreted as a REPL command, not as SQL. A recognized command SHALL perform its action and return to the prompt. An unrecognized `/<word>` SHALL print a friendly error naming the unknown command and pointing to `/help`, then return to the prompt; the REPL SHALL NOT exit and the line SHALL NOT be sent to the SQL engine. Non-slash input SHALL continue to be executed as SQL exactly as before.

#### Scenario: Unknown slash command prints guidance and continues
- **WHEN** the user types `/foo` and presses Enter
- **THEN** the REPL prints `unknown command /foo, try /help` to stderr and the `sql> ` prompt reappears without exiting

#### Scenario: Leading whitespace before a slash is tolerated
- **WHEN** the user types `   /help` and presses Enter
- **THEN** the REPL runs the `/help` command

#### Scenario: Non-slash input is still executed as SQL
- **WHEN** the user types `SELECT name FROM pods LIMIT 1` and presses Enter
- **THEN** the line is executed as a SQL query (not treated as a command)

### Requirement: REPL provides a /clear command
The `/clear` command SHALL clear the terminal screen, leaving the in-memory session history intact. When stdin is not a TTY, `/clear` SHALL be a no-op (no escape codes emitted).

#### Scenario: /clear clears the screen and keeps history
- **WHEN** the user runs `/clear` in a TTY after executing queries
- **THEN** the terminal screen is cleared and pressing up-arrow still recalls a previously entered query

#### Scenario: /clear is a no-op off a TTY
- **WHEN** `/clear` is encountered in non-interactive (piped) input
- **THEN** nothing is written for it and processing continues to the next line

### Requirement: REPL provides a /history-clear command
The `/history-clear` command SHALL clear the REPL's in-memory command history so previously entered queries are no longer recalled by the up/down arrows. The terminal screen is left untouched. When stdin is not a TTY (no history exists), `/history-clear` SHALL be a no-op.

#### Scenario: /history-clear empties the recall history
- **WHEN** the user runs `/history-clear` in a TTY after executing queries
- **THEN** pressing up-arrow no longer recalls a previously entered query

#### Scenario: /history-clear is a no-op off a TTY
- **WHEN** `/history-clear` is encountered in non-interactive (piped) input
- **THEN** nothing happens and processing continues to the next line

### Requirement: REPL provides a /version command
The `/version` command SHALL print the build version string and the project URL `https://github.com/ebuildy/kubectl-sql` to stdout. When no version is injected at build time, the version string SHALL default to `dev`.

#### Scenario: /version prints version and project URL
- **WHEN** the user runs `/version`
- **THEN** the REPL prints the version string and `https://github.com/ebuildy/kubectl-sql`

#### Scenario: Default version when not injected
- **WHEN** the binary was built without version injection and the user runs `/version`
- **THEN** the printed version string is `dev`

### Requirement: REPL provides a /tables command
The `/tables` command SHALL list the queryable tables by dispatching the same code path as the `SHOW TABLES` statement, producing identical output.

#### Scenario: /tables lists tables like SHOW TABLES
- **WHEN** the user runs `/tables`
- **THEN** the REPL prints the same table listing that `SHOW TABLES` produces

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

### Requirement: Tab completes function names
In the interactive prompt, pressing Tab SHALL offer completions for SQL function names — both octosql's built-in functions (e.g. `upper`, `lower`, `coalesce`) and kubectl-sql's custom functions (`length`, `contains`, `keys`, `map_get`, `map_contains_key`, `map_values`). Function names SHALL be offered alongside keyword and column candidates wherever an expression is expected (e.g. after `SELECT`, `WHERE`, or inside argument lists), matched case-insensitively against the typed prefix. Function names are stored and matched in lowercase, and SHALL participate in the same alphabetical ordering and 50-entry cap as other candidates. Each function completion SHALL be suffixed with `(`, since a function name is always followed by its argument list.

#### Scenario: Custom function name completes
- **WHEN** the user types `SELECT map_g` and presses Tab
- **THEN** the completer offers `map_get(`

#### Scenario: Built-in octosql function name completes
- **WHEN** the user types `SELECT upp` and presses Tab
- **THEN** the completer offers `upper(`

#### Scenario: Function names mix with keyword and column candidates
- **WHEN** the user types `SELECT l` against a query whose `FROM` clause is `pods` and presses Tab
- **THEN** the completer offers a combined, alphabetically-sorted list that may include the `length` function, the `LIKE`/`LIMIT` keywords, and any matching column names (e.g. `labels`)

#### Scenario: Function name completion is case-insensitive
- **WHEN** the user types `MAP_C` and presses Tab
- **THEN** the completer offers a completion for `map_contains_key`, matched case-insensitively against the typed prefix

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

### Requirement: Tab completes slash commands
In the interactive prompt, when the word being completed begins with `/`, pressing Tab SHALL offer REPL slash-command names (`/quit`, `/clear`, `/history-clear`, `/help`, `/version`, `/tables`) matched case-insensitively against the typed prefix. Slash-command completion SHALL be mutually exclusive with SQL keyword, table, column, and function completion — a `/`-prefixed word SHALL NOT produce any SQL candidates. Candidates SHALL be ordered alphabetically and capped consistent with other completion (50 entries).

#### Scenario: Bare slash offers all commands
- **WHEN** the user types `/` and presses Tab
- **THEN** the completer offers `/clear`, `/help`, `/history-clear`, `/quit`, `/tables`, `/version`

#### Scenario: Slash prefix narrows to one command
- **WHEN** the user types `/cl` and presses Tab
- **THEN** the completer offers `/clear`

#### Scenario: Slash completion excludes SQL candidates
- **WHEN** the user types `/s` and presses Tab
- **THEN** the completer offers only slash commands (e.g. none match `/s`, so no candidates) and never SQL keywords like `SELECT`

### Requirement: REPL pre-fills a typo correction into the input line
The REPL SHALL, when a query entered at the prompt fails because of a single mistyped keyword, table name, or field and a close valid match is found, present a correction governed by the `query-typo-suggestion` capability WITHOUT prompting for yes/no confirmation. The REPL SHALL print a diagnostic line naming the mistyped token and the suggested token (e.g. `error: field staus does not exist, did you mean status?`) and SHALL pre-fill the corrected query into the next input line, leaving the cursor positioned so the user can press Enter to run it as-is or edit it before running. The corrected query SHALL NOT run until the user submits the (possibly edited) input line. When no close match exists the REPL SHALL print the original error and return to the prompt as before. The REPL SHALL NOT exit in any of these cases.

#### Scenario: REPL suggests a field fix by pre-filling the input
- **WHEN** the user types `SELECT staus FROM pods` at the REPL and `status` is the closest valid field
- **THEN** the REPL prints `error: field staus does not exist, did you mean status?`, pre-fills `SELECT status FROM pods` into the next prompt for editing, and does not run it until the user presses Enter

#### Scenario: REPL suggests a keyword fix by pre-filling the input
- **WHEN** the user types `SLECT name FROM pods` at the REPL
- **THEN** the REPL prints a keyword diagnostic (`did you mean SELECT?`) and pre-fills `SELECT name FROM pods` into the next prompt for editing

#### Scenario: User edits the pre-filled query before running
- **WHEN** the corrected query is pre-filled and the user edits it before pressing Enter
- **THEN** the edited query (not the original suggestion) is the one executed

#### Scenario: User clears the pre-filled query
- **WHEN** the corrected query is pre-filled and the user clears the line instead of running it
- **THEN** the corrected query is not executed and the REPL remains at the prompt

#### Scenario: REPL typo with no close match prints the error and continues
- **WHEN** a REPL query has a mistyped token with no valid match within the similarity threshold
- **THEN** the REPL prints the original error and returns to the `sql> ` prompt without exiting

#### Scenario: Batch mode prints the suggestion without running it
- **WHEN** queries are piped into the REPL (non-interactive batch mode) and one has a single typo with a close match
- **THEN** the REPL prints the full suggestion line (including the corrected query) and continues to the next query without running the correction (no editable prompt exists off a TTY)
