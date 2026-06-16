## Why

The REPL currently uses a psql-style backslash command convention (`\q`, `\help`, `?`) that is inconsistent with the slash-command convention users now expect from modern interactive shells. The command surface is also thin — there is no way to clear the screen, see the version, or list tables without typing full SQL. Migrating to slash commands and adding a small, discoverable command set makes the REPL more intuitive and self-documenting.

## What Changes

- **BREAKING**: Replace backslash commands with slash commands. `\q`, `\help`, and `?` stop working.
- Add `/quit` — exit with code 0. Bare-word `quit` and `exit` (case-insensitive, no prefix) remain as convenience aliases that also exit.
- Add `/clear` — clear the terminal screen (like Ctrl-L); in-memory session history is preserved. No-op when stdin is not a TTY.
- Add `/help` — print the list of available slash commands (replaces `\help` / `?`).
- Add `/history-clear` — must clear history.
- Add `/version` — print the version string and the project URL `https://github.com/ebuildy/kubectl-sql`.
- Add `/tables` — alias that dispatches the existing `SHOW TABLES` code path, producing identical output.
- Add a dispatch rule: any input line whose first non-space character is `/` is a REPL command, not SQL. Unknown `/foo` prints a friendly error (`unknown command /foo, try /help`), the prompt reappears, and the REPL does not exit.
- Add Tab completion for slash commands: a `/`-prefixed word offers REPL command names (mutually exclusive with SQL keyword/table/column/function completion).
- Add build-time version injection infrastructure (`var version = "dev"` via `-ldflags`, Makefile target updated).

Ctrl-C / Ctrl-D behavior is unchanged.

## Capabilities

### New Capabilities
<!-- None -->

### Modified Capabilities
- `sql-repl`: Replaces the backslash exit/help command requirements with slash-command equivalents; adds `/clear`, `/version`, `/tables`, a slash-command dispatch rule, and slash-command Tab completion. Retains bare-word `quit`/`exit` as exit aliases.

## Impact

- **Code**: `internal/domain/commands/repl/command.go` (command dispatch), `internal/adapter/shell/readline/repl.go` (prompt loop, `/clear` screen handling), `internal/adapter/shell/completion` (slash-command completer), `cmd/root.go` (version variable, project URL constant).
- **Build**: `Makefile` build target updated to inject the version via `-ldflags "-X ...version=..."`.
- **Interface (BREAKING)**: documented REPL commands `\q`, `\help`, `?` removed. Users relying on them must switch to `/quit`, `/help`. Long-lived `sql-repl` spec reconciled on archive.
- **Reuse**: `/tables` dispatches the existing `show-tables` capability; no new listing logic.
