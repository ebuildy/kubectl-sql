## Why

Running one-off SQL queries via `kubectl sql "<query>"` requires escaping, quote juggling, and repeated binary invocations. A REPL lets users iterate interactively, build queries incrementally, and get instant feedback — making exploratory debugging significantly faster.

## What Changes

- Add a `--repl` / `-i` flag (or bare `kubectl sql` with no positional argument) that opens an interactive Read-Eval-Print-Loop
- Display a prompt (`kuery> `) and accept multi-line SQL terminated by `;` or Enter on an empty line
- Execute the query on Enter, print results in the configured output format, then return to the prompt
- `\q`, `quit`, `exit`, or Ctrl-C exits cleanly
- `\help` or `?` prints available slash commands
- When invoked with an empty query argument (no positional arg and no `--repl` flag), fall into REPL mode automatically
- REPL inherits all CLI flags: `--output`, `--namespace`, `--context`, `--kubeconfig`, etc.
- **Tab autocomplete** in the interactive prompt:
  - SQL keywords (`SELECT`, `FROM`, `WHERE`, `ORDER BY`, `LIMIT`, `GROUP BY`, …)
  - Table names (queryable Kubernetes resources, from discovery — same source as `SHOW TABLES`)
  - Column names for the table named in the current `FROM` clause (from the schema inferrer)
  - Completion preserves the case the user typed; table schema is prefetched and cached when a table appears in a completed `FROM` clause so column completion is instant

## Capabilities

### New Capabilities

- `sql-repl`: Interactive REPL mode — prompt loop, query input, result rendering, slash commands, clean exit

### Modified Capabilities

- `sql-execution`: Entry point behaviour changes — no positional argument now opens REPL instead of showing help

## Impact

- `cmd/root.go`: entry-point logic branches into REPL mode when no query argument is provided
- New package `internal/repl/` with the prompt loop and a `readline.AutoCompleter` implementation
- Dependency: `github.com/chzyer/readline` (or `golang.org/x/term` raw-mode) for line editing, history, and Tab completion
- Completion reuses existing infrastructure: discovery (`ServerPreferredResources`) for table names and `internal/schema.SchemaInferrer` for column names — injected into the REPL so the package stays cmd-independent
- `openspec/specs/sql-execution/spec.md`: "missing argument shows help" scenario changes to "missing argument opens REPL"
