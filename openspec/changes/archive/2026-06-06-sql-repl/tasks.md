## 1. Dependency

- [x] 1.1 Add `github.com/chzyer/readline` to `go.mod` and `go.sum` via `go get`

## 2. REPL Package

- [x] 2.1 Create `internal/repl/repl.go` with `Run(ctx context.Context, cfg Config, w io.Writer) error` — `Config` holds the kubeconfig/context/namespace/output/page-size/timeout/no-color values forwarded from CLI flags
- [x] 2.2 Implement TTY detection inside `Run`: if `term.IsTerminal(int(os.Stdin.Fd()))` is false, delegate to `runBatch` (line-by-line stdin, no prompt)
- [x] 2.3 Implement `runBatch`: read lines from `os.Stdin` with `bufio.Scanner`, skip empty lines, execute each query via `runQueryWithWriter`, stop on EOF
- [x] 2.4 Implement `runInteractive`: open a `readline.NewEx` instance with prompt `"sql> "` and in-memory history; loop reading lines
- [x] 2.5 Handle slash commands in the interactive loop: `\q` / `quit` / `exit` → return nil (exit 0); `\help` / `?` → print help text to w
- [x] 2.6 Strip trailing `;` from query string before executing
- [x] 2.7 Execute each non-empty, non-slash-command line by calling the existing `runQueryWithWriter` helper (to be refactored/exported from `cmd/root.go` if needed)
- [x] 2.8 On query error, print to stderr and return to prompt (do not exit)
- [x] 2.9 Handle SIGINT (Ctrl-C) during a running query: cancel the per-query context, print `^C`, return to prompt; SIGINT at the idle prompt exits cleanly

## 3. Entry-Point Integration

- [x] 3.1 Add `--repl` / `-i` boolean flag to `cmd/root.go`
- [x] 3.2 In `cmd/root.go` `RunE`: if no positional args provided AND (stdin is TTY OR `--repl` flag is set), call `repl.Run(...)` instead of printing help
- [x] 3.3 Export or refactor `runQueryWithWriter` so it can be called from `internal/repl` without import cycle (move to `internal/query` or pass as a function parameter)

## 4. Tests

- [x] 4.1 Unit test `runBatch` with a pipe of two queries — assert both execute and output is written to `w`
- [x] 4.2 Unit test slash command handling: `\q` returns nil, `\help` writes help text, empty line is a no-op
- [x] 4.3 Unit test `;` stripping: `"SELECT name FROM pods;"` → query passed to executor is `"SELECT name FROM pods"`
- [x] 4.4 Add integration feature scenario: `echo "SELECT name FROM pods LIMIT 1" | kubectl-sql` exits 0 and produces JSON output (non-TTY batch mode)

## 5. Documentation

- [x] 5.1 Update `README.md` usage section: add REPL invocation example and flag table entry for `--repl` / `-i`

## 6. Autocomplete

- [x] 6.1 Create `internal/repl/complete.go` with a `completer` type implementing `readline.AutoCompleter` (`Do(line []rune, pos int) ([][]rune, int)`)
- [x] 6.2 Add a `Completion` source struct/interface to `Config`: `Tables() []string` (resource names) and `Columns(table string) []string` (column names), so the REPL package stays cmd-independent
- [x] 6.3 Implement keyword completion: match the word under the cursor against a static keyword list (`SELECT`, `FROM`, `WHERE`, `ORDER BY`, `LIMIT`, `GROUP BY`, `DISTINCT`, `AS`, `AND`, `OR`, `NOT`, `IN`, `IS NULL`, `LIKE`), preserving the typed case (lower prefix → lower candidate, upper prefix → upper candidate)
- [x] 6.4 Implement table-name completion: when the word under the cursor follows `FROM`/`JOIN`, complete against `Completion.Tables()`
- [x] 6.5 Parse the current line to extract the `FROM <table>` token; implement column completion against `Completion.Columns(table)` when the cursor is in a column position
- [x] 6.6 Eager prefetch + cache: when a table is resolved from the line, populate the column cache for that table once (via the inferrer) so completion is instant thereafter; guard with a mutex for the per-query goroutine
- [x] 6.7 Wire the completer into `readline.NewEx` via `Config.AutoComplete`; only enable in interactive mode
- [x] 6.8 In `cmd/root.go`, build the `Completion` source: `Tables()` from `discoClient.ServerPreferredResources()` (same filter as `runShowTables`), `Columns()` from `internalschema` inferrer keyed by resource; pass it into `repl.Config`
- [x] 6.9 Unit test keyword completion (case preservation), table completion after `FROM`, and column completion using a fake `Completion` source; test that an unresolved table yields no column candidates
- [x] 6.10 Update `README.md`: note Tab autocomplete for keywords, tables, and columns in the REPL section
