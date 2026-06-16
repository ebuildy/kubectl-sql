## ADDED Requirements

### Requirement: A failed query produces a single-token correction suggestion
The engine SHALL, when a query fails because of a single mistyped token — a SQL keyword, a table name, or a field name — for which a close valid alternative exists, produce a structured correction suggestion instead of only returning the raw error. The suggestion SHALL carry the typo token, its kind (keyword / table / field / syntax), the suggested valid token, and the corrected query text. Detection SHALL cover the pipeline stages, in this order of precedence: (1) a parse failure caused by an unterminated string literal (a missing closing quote) — see the dedicated requirement below; (2) a parse failure caused by a mistyped keyword; (3) a table-resolution failure caused by a mistyped resource name (octosql `resolve resource "<name>"`); (4) a typecheck failure caused by a mistyped field (octosql `unknown variable: '<name>'` or `object field access of field '<field>' ... without that field`). A failure that does not reduce to one of these cases (e.g. a genuine syntax error with no close keyword and balanced quotes, an unknown function) SHALL NOT produce a suggestion.

#### Scenario: Keyword typo is detected
- **WHEN** the user runs `SLECT name FROM pods`
- **THEN** a suggestion of kind keyword is produced with suggested token `SELECT` and corrected query `SELECT name FROM pods`

#### Scenario: Table-name typo is detected
- **WHEN** the user runs `SELECT name FROM pdos`
- **THEN** a suggestion of kind table is produced with suggested token `pods` and corrected query `SELECT name FROM pods`

#### Scenario: Field typo is detected
- **WHEN** the user runs `SELECT staus FROM pods` and `status` is a valid field of `pods`
- **THEN** a suggestion of kind field is produced with suggested token `status` and corrected query `SELECT status FROM pods`

#### Scenario: A failure with no close match produces no suggestion
- **WHEN** the user runs a query whose failing token has no valid alternative within the similarity threshold
- **THEN** no suggestion is produced and the original error is returned

### Requirement: An unterminated string literal is corrected by closing the quote
For a parse failure where the query contains a string literal opened with `'` or `"` that is never closed, the engine SHALL propose a correction that is the original query text with the matching closing quote appended. This case (kind `syntax`) SHALL take precedence over keyword detection on a parse failure, since an open quote makes the remainder of the query un-lexable. The suggestion SHALL be offered only when appending the closing quote makes the query parse successfully; otherwise no suggestion is produced. A backslash-escaped quote inside a literal SHALL NOT be treated as closing it. Only one closing quote SHALL be added per attempt; any further typo (e.g. a mistyped field in the now-parseable query) surfaces on the next run.

#### Scenario: Missing closing double quote is corrected
- **WHEN** the user runs `select status from po where nam = "toto` (no closing quote)
- **THEN** a suggestion of kind syntax is produced with corrected query `select status from po where nam = "toto"`

#### Scenario: Missing closing single quote is corrected
- **WHEN** the user runs `select status from po where nam = 'toto` (no closing quote)
- **THEN** the corrected query is `select status from po where nam = 'toto'`

#### Scenario: Balanced quotes are not flagged
- **WHEN** a query's string literals are all properly closed but it fails to parse for another reason
- **THEN** no unterminated-quote suggestion is produced

### Requirement: Keyword typos are corrected against the supported SQL keyword set
For a parse failure, the candidate set SHALL be the SQL keywords kubectl-sql supports (the same set offered by Tab completion). The suggestion SHALL replace the first bareword token (outside any quoted string) whose closest keyword match clears the similarity threshold and which is not already an exact keyword. Quoted string literals SHALL never be altered.

#### Scenario: Mistyped statement keyword is corrected
- **WHEN** the user runs `SLECT name FROM pods`
- **THEN** `SLECT` is corrected to `SELECT` and the rest of the query is preserved

#### Scenario: Mistyped clause keyword is corrected
- **WHEN** the user runs `SELECT name FORM pods`
- **THEN** `FORM` is corrected to `FROM`

#### Scenario: Quoted literals are never treated as keyword typos
- **WHEN** a query contains a quoted string that resembles a keyword (e.g. `WHERE name = 'slect'`)
- **THEN** the quoted literal is left unchanged

### Requirement: Table-name typos are corrected against queryable resources
For a table-resolution failure, the candidate set SHALL be the names of the cluster's queryable resource types (the same list backing `SHOW TABLES` and Tab completion). Accepted plural forms and shortnames SHALL NOT be treated as typos. The suggestion SHALL replace the mistyped resource token in the `FROM` (or `JOIN`) position.

#### Scenario: Mistyped resource name is corrected
- **WHEN** the user runs `SELECT name FROM pdos` and `pods` is a queryable resource
- **THEN** `pdos` is corrected to `pods`

#### Scenario: Valid shortname is not flagged
- **WHEN** the user runs `SELECT name FROM po` and `po` is an accepted shortname for pods
- **THEN** the query resolves normally and no suggestion is produced

### Requirement: Field candidates are scoped to the parent at the failing position
For a field typecheck failure, the candidate set SHALL be the valid field names available at the exact position where the unknown field was referenced, never the whole table's field list when the typo is nested. For a top-level unknown field (including the base segment of a `->` chain) the candidates SHALL be the table's top-level schema field names. For a nested `->` field whose parent segments are valid, the candidates SHALL be the subfields of the parent resolved by walking the `->` chain that precedes the failing segment down the inferred schema (including a list element's struct subfields for `list[index]->field` access). If the parent reported by typecheck cannot be resolved against the inferred schema, no candidate set SHALL be available and no suggestion SHALL be produced for that run.

#### Scenario: Top-level field typo uses the table's top-level fields
- **WHEN** the user runs `SELECT staus FROM pods`
- **THEN** the candidates are the table's top-level field names and `status` is suggested

#### Scenario: Nested field typo uses only the parent struct's subfields
- **WHEN** the user runs `SELECT spec->contenairs FROM pods` and `spec` has a subfield `containers`
- **THEN** only the subfields under `spec` are considered as candidates, `containers` is suggested, and unrelated top-level or `status` fields are not offered

#### Scenario: Deep nested field typo uses the list-element struct's subfields
- **WHEN** the user runs `SELECT spec->containers[0]->imagee FROM pods` and a container has a subfield `image`
- **THEN** the candidates are the container element struct's subfields and `image` is suggested

### Requirement: Dotted sub-field access is corrected to the arrow operator
When a typecheck failure reports an unknown variable whose name contains a dot (e.g. `unknown variable: 'spec.annotations'`), the engine SHALL interpret it as dotted sub-field access that should use the `->` operator, rather than a mistyped field. The suggestion (kind `dot-notation`) SHALL convert the full contiguous dotted identifier chain in the query that contains the reported token to the arrow form (`spec.annotations` → `spec->annotations`, `metadata.labels.app` → `metadata->labels->app`), and its message SHALL remind the user that object sub-fields are accessed with `->` rather than `.`. octosql may report only the trailing segments of the chain (e.g. `labels.app` for `metadata.labels.app`); the suggestion SHALL still convert the whole chain so the correction is complete in one step, and SHALL NOT alter unrelated dotted tokens elsewhere in the query (e.g. a `notes.json` file source).

#### Scenario: Dotted sub-field access becomes arrow access
- **WHEN** the user runs `select spec.annotations from pod`
- **THEN** a suggestion of kind dot-notation is produced with corrected query `select spec->annotations from pod` and a message reminding that `->` (not `.`) accesses object sub-fields

#### Scenario: A multi-level dotted chain is fully converted
- **WHEN** the user runs `select metadata.labels.app from pods`
- **THEN** the corrected query is `select metadata->labels->app from pods`

### Requirement: Closest valid token is chosen by string similarity within a threshold
The suggested token SHALL be the candidate with the highest case-insensitive string-similarity score to the typo token, as determined by the `spellchecker` port, and selected only when that score meets the port's configured threshold. The threshold SHALL be conservative — a wrong suggestion is worse than none — so only high-confidence single-typo corrections are offered and vaguely-similar tokens are left unsuggested. A candidate SHALL additionally be rejected when it is more than 30% longer than the typo token (measured in characters), with an absolute floor of at least one extra character so common single-character additions still pass — a correction is roughly the same length as the typo, never a wildly longer unrelated token. When no candidate satisfies both the threshold and the length bound, no suggestion SHALL be produced and the original error SHALL be returned unchanged. Ties in score SHALL be broken deterministically (shortest candidate, then lexicographic order).

#### Scenario: Closest token within threshold is suggested
- **WHEN** the typo is `staus` and the candidates include `status` (a high-similarity match)
- **THEN** `status` is suggested

#### Scenario: No close match yields no suggestion
- **WHEN** the typo is `xyzzy` and no candidate is within the similarity threshold
- **THEN** no suggestion is produced and the original error is returned

#### Scenario: A much longer candidate is never suggested
- **WHEN** the typo is `toot` and the only same-ish candidates are far longer (e.g. `replicationcontrollers`)
- **THEN** no suggestion is produced (the candidate exceeds the 30% length bound) and the original error is returned

### Requirement: Only one token is corrected per attempt
A single suggestion SHALL correct exactly one token — the first mistyped token in pipeline-precedence order (keyword, then table, then field). The corrected query SHALL be the original query text with only that one token substituted (whole-word / whole-identifier, first occurrence). If the corrected query still contains another typo, that next typo SHALL only be addressed on a subsequent run, never bundled into the same suggestion.

#### Scenario: Two field typos are corrected one at a time
- **WHEN** the user runs `SELECT staus, nme FROM pods` where both `staus` and `nme` are typos
- **THEN** the first suggestion corrects only one field (e.g. `SELECT status, nme FROM pods`), leaving the second typo for the next run

#### Scenario: Typos across stages are corrected by precedence
- **WHEN** the user runs `SLECT name FROM pdos` with both a keyword and a table typo
- **THEN** the keyword is corrected first (`SELECT name FROM pdos`), and the table typo surfaces on the next run

#### Scenario: Base-segment field typo in a chain is corrected first
- **WHEN** the user runs `SELECT spc->contenairs FROM pods` where both `spc` and `contenairs` are typos
- **THEN** typecheck reports the base `spc` first, the correction is `SELECT spec->contenairs FROM pods`, and `contenairs` is left for the next run

#### Scenario: Substitution preserves the rest of the query verbatim
- **WHEN** the corrected token is substituted into `SELECT staus FROM pods WHERE name = 'x'`
- **THEN** only `staus` is replaced and the `WHERE name = 'x'` clause is preserved unchanged

### Requirement: Correction is confirmed before the corrected query runs
The corrected query SHALL never be executed without an explicit user action; the confirmation mechanism depends on the surface:

- **One-shot CLI (TTY):** the suggestion SHALL be presented in a kind-appropriate message ending with the corrected query, e.g. `error: field staus does not exist, run this query instead ? SELECT status FROM pods`, and the user SHALL be prompted to confirm with a default answer of yes (pressing Enter runs the corrected query). On confirmation the corrected query SHALL be executed in place of the failed one; on rejection it SHALL NOT run.
- **REPL (interactive):** the REPL SHALL NOT prompt yes/no. It SHALL print a diagnostic naming the mistyped and suggested tokens (e.g. `error: field staus does not exist, did you mean status?`) and pre-fill the corrected query into the next input line for editing; the corrected query runs only when the user submits that (possibly edited) line. This behaviour is specified by the `sql-repl` capability.
- **Non-interactive (piped stdin / batch):** the suggestion line SHALL be printed but the corrected query SHALL NOT be executed automatically.

#### Scenario: One-shot user accepts the suggestion
- **WHEN** an interactive one-shot user is shown the suggestion and presses Enter (or answers yes)
- **THEN** the corrected query is executed and its results are rendered

#### Scenario: One-shot user rejects the suggestion
- **WHEN** an interactive one-shot user is shown the suggestion and answers no
- **THEN** the corrected query is not executed

#### Scenario: Non-interactive session never auto-runs the correction
- **WHEN** a query with a single typo is run with piped stdin
- **THEN** the suggestion line is printed and the corrected query is not executed
