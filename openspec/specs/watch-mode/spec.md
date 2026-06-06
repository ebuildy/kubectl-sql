## MODIFIED Requirements

### Requirement: --watch flag re-executes the query on a polling interval
When `--watch` / `-w` is passed, `kubectl-sql` SHALL re-execute the full SQL query every 5 seconds and print the result table to stdout, identical to a normal batch execution. Each iteration clears the previous output and reprints the full table, giving a live-refreshing view. The polling loop runs until the user presses Ctrl-C (SIGINT) or `--timeout` is reached.

There are no Kubernetes WATCH/event semantics — this is pure polling: each tick runs the full octosql pipeline (schema inference → typecheck → LIST → filter → render) and replaces the previous output.

#### Scenario: Watch re-executes and reprints the table
- **WHEN** the user runs `kubectl sql --watch "SELECT name, namespace, status->phase FROM pods"`
- **THEN** the command prints the full result table, waits 5 seconds, clears the screen, and reprints the updated table, repeating until Ctrl-C

#### Scenario: Watch output is identical to batch output
- **WHEN** the same query is run with and without `--watch`
- **THEN** the table columns, values, and formatting are identical; watch adds only the periodic refresh behaviour

#### Scenario: Watch respects all SQL clauses
- **WHEN** the user runs `kubectl sql --watch "SELECT name FROM pods ORDER BY name LIMIT 10"`
- **THEN** the query runs normally with ORDER BY and LIMIT applied on every tick

#### Scenario: Watch exits cleanly on SIGINT
- **WHEN** the user presses Ctrl-C
- **THEN** the command exits 0

#### Scenario: Watch respects --timeout
- **WHEN** the user runs `kubectl sql --watch --timeout 10s "SELECT name FROM pods"`
- **THEN** the command stops after 10 seconds and exits 0

#### Scenario: Watch respects --namespace and --output flags
- **WHEN** the user runs `kubectl sql --watch -n kube-system --output json "SELECT name FROM pods"`
- **THEN** each tick outputs a fresh JSON array for the kube-system namespace
