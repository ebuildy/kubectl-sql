## 1. Version infrastructure

- [x] 1.1 Add a package-level `var version = "dev"` and a `const projectURL = "https://github.com/ebuildy/kubectl-sql"` in `cmd/root.go` (or a small dedicated file in `cmd/`).
- [x] 1.2 Thread `version` and project URL down to the REPL: add `Version string` and `ProjectURL string` (or a single version-info value) to `NewReadlineShell` and populate them from `cmd` when constructing the shell via `ReplCommand.Run` (avoid importing `cmd` from `repl` — pass values in).
- [x] 1.3 Update the Makefile `build` target to inject the version with `-ldflags "-X github.com/ebuildy/kubectl-sql/cmd.version=<value>"` (e.g. from `git describe --tags --always --dirty`); plain `go build` SHALL still default to `dev`.

## 2. Slash-command dispatch

- [x] 2.1 In `internal/adapter/shell/readline/repl.go`, change the dispatch rule so any input whose first non-space char is `/` is routed to the command handler (not SQL); keep bare-word `quit`/`exit` (case-insensitive) as exit aliases. Remove `\q`, `\help`, `?` recognition.
- [x] 2.2 Rework `handleSlashCommand` into a method/handler with access to what it needs (ctx, RunQuery, writer, IsTTY, version/URL) to support `/quit`, `/clear`, `/help`, `/version`, `/tables`.
- [x] 2.3 Implement `/quit` → exit code 0; bare `quit`/`exit` continue to exit.
- [x] 2.4 Implement `/help` → print the slash-command list (`/quit`, `/clear`, `/help`, `/version`, `/tables`); replace the old `printHelp` text.
- [x] 2.5 Implement `/version` → print version string and project URL; default version `dev` when not injected.
- [x] 2.6 Implement `/tables` → dispatch `SHOW TABLES` through `RunQuery` so output is identical to the SQL statement.
- [x] 2.7 Implement `/clear` → clear the terminal screen (ANSI/readline clear) when `IsTTY`; no-op and emit nothing when not a TTY; preserve in-memory history.
- [x] 2.7b Implement `/history-clear` → reset the readline in-memory history (`rl.ResetHistory`) so up/down no longer recall prior queries; no-op off a TTY; screen untouched.
- [x] 2.8 Unknown `/foo` → print `unknown command /foo, try /help` to stderr, return to prompt, do not exit, do not send to SQL engine.
- [x] 2.9 In batch mode (`runBatch`), make `/clear` a no-op and keep `quit`/`exit`/`/quit` exiting; ensure other commands behave consistently off a TTY.

## 3. Tab completion for slash commands

- [x] 3.1 In `internal/adapter/shell/completion/complete.go`, detect when the word under completion begins with `/` and return only slash-command candidates (`/quit`, `/clear`, `/help`, `/version`, `/tables`), matched case-insensitively, alphabetically ordered, capped consistent with the existing 50-entry cap.
- [x] 3.2 Ensure slash-command completion is mutually exclusive with SQL keyword/table/column/function completion (a `/`-prefixed word yields no SQL candidates); `/` alone offers all five commands.

## 4. Tests

- [x] 4.1 Update/extend `repl` tests for the new dispatch: `/quit` and bare `quit`/`exit` exit; `\q`/`\help`/`?` no longer recognized (print error, no exit).
- [x] 4.2 Add tests for `/help`, `/version` (incl. default `dev`), `/tables` (matches `SHOW TABLES`), `/clear` (no-op off TTY), and unknown `/foo` behavior.
- [x] 4.3 Add completion tests in `complete_test.go`: `/` offers all commands, `/cl` → `/clear`, slash word excludes SQL candidates.

## 5. Docs & verification

- [x] 5.1 Update the README REPL section to document the slash commands (note the breaking removal of `\q`/`\help`/`?`).
- [x] 5.2 Run `make lint build` and `make test` until clean.
