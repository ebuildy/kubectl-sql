## Why

When a user mistypes a token in a query — a SQL keyword (`SLECT`), a table name (`pdos`), or a
field (`staus`) — the query fails with a raw error that gives no hint about the intended token.
kubectl-sql already knows the valid alternatives for each: the supported keyword set, the cluster's
queryable resources, and the inferred schema fields. We can detect the typo, find the closest valid
token, and offer to run the corrected query — turning a dead end into a one-keystroke fix.

## What Changes

- Add single-token typo detection across the pipeline stages: an **unterminated string literal** and
  a mistyped **keyword** (both parse failures), a mistyped **table name** (resource-resolution
  failure), and a mistyped **field** (typecheck failure). For the token cases, compare the failing
  token against the relevant valid set using a string similarity library and pick the closest match
  within a threshold; for the unterminated literal, append the matching closing quote (a structural
  fix verified by re-parsing).
- Replace the raw error with a corrective message ending in the corrected query, e.g.
  `error: field staus does not exist, run this query instead ? SELECT status FROM pods`, where the
  suggestion is the original query with the single mistyped token substituted.
- Add a confirmation step whose mechanism depends on the surface: on the one-shot CLI the user is
  prompted (default yes) to run the corrected query; in the REPL there is no yes/no prompt — the
  corrected query is pre-filled into the next input line so the user can press Enter to run it or
  edit it first. In a non-interactive session the suggestion is printed and the command exits
  non-zero without auto-running it.
- Handle **one** typo per attempt only, in precedence order (keyword → table → field); any remaining
  typo is offered on the following run.
- Applies to both the one-shot CLI path and the REPL.

## Capabilities

### New Capabilities
- `query-typo-suggestion`: detecting a single mistyped keyword, table name, or field across the
  parse / table-resolution / typecheck stages, ranking candidate corrections by string similarity
  against the relevant valid set, building the corrected query, prompting for confirmation
  (interactive vs non-interactive behavior), and the single-typo-at-a-time precedence rule.

### Modified Capabilities
- `sql-execution`: the "Invalid SQL prints an error / exits 1" path is augmented — when the failure
  is a single unknown-field typo with a close schema match, the command emits a correction
  suggestion (and, when confirmed interactively, runs the corrected query) instead of only erroring.
- `sql-repl`: the REPL's invalid-query handling offers the suggestion at the prompt and re-runs the
  corrected query on confirmation before returning to `sql> `.

## Impact

- New port `internal/port/spellchecker` (similarity interface) and adapter
  `internal/adapter/spellchecker` (the only package importing `github.com/adrg/strutil`), following
  the hexagonal logger-port / zap-adapter convention.
- Code: `internal/adapter/sql/octosql` (engine parse/typecheck error paths, schema field
  enumeration, resource-resolution error; takes an injected `SpellChecker`),
  `internal/domain/commands/query` (wires the spellchecker adapter + one-shot confirmation prompt),
  `internal/adapter/shell/readline` (REPL input-line pre-fill of the corrected query).
- Reuses three existing valid-token sources: the supported SQL keyword list and queryable resource
  list already used by Tab completion / `SHOW TABLES` (`internal/adapter/shell/completion`), and the
  field names produced by the dynamic-schema inferrer.
- Adds one new dependency: `github.com/adrg/strutil` for string similarity scoring, confined to the
  spellchecker adapter (justified in `design.md`).
- No change to write semantics; this is read-path UX only.
