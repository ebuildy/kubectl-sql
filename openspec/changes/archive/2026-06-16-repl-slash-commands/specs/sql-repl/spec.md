## MODIFIED Requirements

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

## ADDED Requirements

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

### Requirement: Tab completes slash commands
In the interactive prompt, when the word being completed begins with `/`, pressing Tab SHALL offer REPL slash-command names (`/quit`, `/clear`, `/help`, `/history-clear`, `/version`, `/tables`) matched case-insensitively against the typed prefix. Slash-command completion SHALL be mutually exclusive with SQL keyword, table, column, and function completion — a `/`-prefixed word SHALL NOT produce any SQL candidates. Candidates SHALL be ordered alphabetically and capped consistent with other completion (50 entries).

#### Scenario: Bare slash offers all commands
- **WHEN** the user types `/` and presses Tab
- **THEN** the completer offers `/clear`, `/help`, `/history-clear`, `/quit`, `/tables`, `/version`

#### Scenario: Slash prefix narrows to one command
- **WHEN** the user types `/cl` and presses Tab
- **THEN** the completer offers `/clear`

#### Scenario: Slash completion excludes SQL candidates
- **WHEN** the user types `/s` and presses Tab
- **THEN** the completer offers only slash commands (e.g. none match `/s`, so no candidates) and never SQL keywords like `SELECT`
