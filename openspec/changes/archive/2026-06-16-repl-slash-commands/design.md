## Context

The REPL (`internal/domain/commands/repl/command.go` + `internal/adapter/shell/readline/repl.go`) currently recognizes a small set of backslash control commands (`\q`, `\help`, `?`) and bare-word exits (`quit`, `exit`), with everything else dispatched to the SQL engine. Tab completion (`internal/adapter/shell/completion`) offers SQL keywords, table names, column names, and function names. There is no version variable anywhere in the codebase and no ldflags injection in the Makefile.

This change swaps the control-command convention from backslash to slash, broadens the command set, and extends Tab completion to slash commands — without touching SQL grammar or output formatting.

## Goals / Non-Goals

**Goals:**
- A single, consistent slash-command convention for REPL control commands.
- Discoverable commands: `/help` lists them; Tab completes them.
- Reuse existing capabilities (`/tables` → `SHOW TABLES`) rather than duplicating logic.
- Build-time version stamping for `/version`.

**Non-Goals:**
- No changes to SQL grammar, parsing, or output renderers.
- No multi-word or argument-taking slash commands (all five are bare verbs).
- No persistent history changes (`/clear` only affects the screen).
- No new external dependencies.

## Decisions

**1. Slash dispatch precedes SQL.** In the REPL eval step, after trimming leading whitespace, if the line begins with `/`, route it to the slash-command handler; otherwise treat it as SQL (after the existing trailing-`;` strip). Bare-word `quit`/`exit` are checked as today, before SQL dispatch, so they keep working. *Alternative considered:* make `quit`/`exit` slash-only — rejected because users type them by habit and the cost of keeping them is one comparison.

**2. Unknown slash commands do not exit and do not reach the SQL engine.** `/foo` prints `unknown command /foo, try /help` to stderr and reprompts. This avoids confusing SQL parse errors for mistyped commands.

**3. `/clear` clears the screen only.** Implemented by writing the terminal clear sequence (or the readline equivalent of Ctrl-L) to stdout; in-memory history is untouched. When stdin is not a TTY, `/clear` is a no-op. *Alternative considered:* clearing history too — rejected as surprising and not requested.

**4. `/tables` dispatches `SHOW TABLES`.** The handler invokes the same code path the SQL engine uses for `SHOW TABLES`, guaranteeing identical output and zero divergence. No new formatting.

**5. Version via ldflags.** Add `var version = "dev"` (package-level, in `cmd/root.go` or a dedicated location referenced by the REPL), overridable at build time with `-ldflags "-X <pkg>.version=<value>"`. The Makefile `build` target injects the value (e.g. from `git describe`). The project URL is a hardcoded constant `https://github.com/ebuildy/kubectl-sql`. Plain `go build` yields `dev`.

**6. Slash Tab completion is mutually exclusive with SQL completion.** When the word under completion starts with `/`, the completer returns only slash-command candidates (matched case-insensitively, alphabetically ordered, capped consistent with the existing 50-entry cap). A `/`-prefixed word never mixes with SQL keyword/table/column/function candidates. `/` alone offers all five commands.

## Risks / Trade-offs

- **[Breaking change: `\q`, `\help`, `?` removed]** → Called out in the proposal and `/help` output; users migrate to `/quit`, `/help`. Bare `quit`/`exit` retained softens the exit-path break.
- **[`/clear` terminal handling varies by terminal]** → Use the readline/ANSI clear convention already viable in the shell adapter; degrade to no-op off a TTY rather than emitting stray escape codes into piped output.
- **[Version not stamped if built via plain `go build`]** → Acceptable; defaults to `dev`. Documented in the Makefile target.
- **[Slash dispatch could shadow a future SQL construct starting with `/`]** → No SQL statement in the supported grammar begins with `/`; low risk.

## Migration Plan

1. Land the dispatch + commands + completion changes behind no flag (REPL is interactive-only surface).
2. Update `/help` text and README REPL docs to show slash commands.
3. On archive, reconcile the long-lived `sql-repl` spec to the slash convention.

No rollback concerns beyond reverting the change; no data or API migration.
