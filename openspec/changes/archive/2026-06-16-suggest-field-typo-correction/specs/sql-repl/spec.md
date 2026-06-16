## ADDED Requirements

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
