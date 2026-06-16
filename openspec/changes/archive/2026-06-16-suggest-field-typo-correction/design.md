## Context

A query can fail at three distinct stages of the octosql pipeline in `engine.Execute`
(`internal/adapter/sql/octosql/engine.go`), each able to be caused by a single mistyped token:

1. **Parse** — `sqlparser.Parse(q.SQL)` returns an error before any rewrite/typecheck. A mistyped
   keyword (`SLECT`, `FORM`) lands here.
2. **Table resolution** — during typecheck, `KubernetesDatabase.GetTable` calls
   `ds.Resolve(ctx, name)`; an unknown resource returns `executor: resolve resource "<name>": …`,
   which octosql surfaces (panic) and the engine recovers as `typecheck error: …`. A mistyped table
   name (`pdos`) lands here.
3. **Field typecheck** — octosql **panics** on an unknown field, recovered into
   `octosql: typecheck: typecheck error: <message>`. Two shapes (from
   `external/octosql/logical/logical.go`): top-level `unknown variable: '<name>'` (e.g. `staus`) and
   nested `object field access of field '<field>' on object expression of type '<type>' without that field`
   (e.g. `status->phse`). Because octosql evaluates a `->` chain from its base, a mistyped base
   segment fails as `unknown variable` (top-level), not object-field-access.

The valid alternatives for each stage are already enumerated for other features:

- **Keywords**: the supported SQL keyword list (`internal/adapter/shell/completion`, `sqlKeywords`).
- **Tables**: queryable resource names (`ds.Resources()`, backing `SHOW TABLES` and completion).
- **Fields**: the schema port (`internal/port/schema`) `[]schema.Field` with `SubFields`, the same
  data the completion source walks (including arrow-chain walking via `arrowChainRe`).

So the data needed to suggest a correction is on hand; what is missing is (1) recognizing each
failure as a single-mistyped-token case and extracting that token, (2) ranking the relevant valid
set by similarity, and (3) offering to re-run a corrected query.

This is a read-path UX change only. It touches the one-shot CLI path
(`internal/domain/commands/query`) and the REPL (`internal/adapter/shell/readline`), both of which
call the same SQL engine.

## Goals / Non-Goals

**Goals:**
- Turn a single mistyped keyword, table name, or field into a concrete, runnable suggestion when a
  close valid match exists.
- Correct exactly one token per attempt (precedence keyword → table → field) and ask for
  confirmation before running the corrected query.
- Work for top-level fields (`staus` → `status`), nested `->` fields (`status->phse` →
  `status->phase`), and list-element fields (`spec->containers[0]->imagee` → `image`).
- Reuse the existing keyword / resource / schema enumerations.

**Non-Goals:**
- Correcting multiple typos in a single pass (explicitly one-at-a-time; the next typo surfaces on
  the next run).
- Correcting function names, or genuine syntax errors with no close keyword match.
- A flag to auto-accept suggestions non-interactively (suggestions are printed but never auto-run
  without confirmation).
- Fuzzy-matching values, label keys, or map keys.

## Decisions

### Decision: Detect the mistyped token from the stage error, not by pre-validating the AST
The pipeline already runs parse, table resolution, and typecheck, each of which authoritatively
knows the valid tokens for its stage. Rather than re-implementing token extraction over the octosql
AST, the suggestion logic triggers **after** a stage fails and parses that stage's error to extract
the offending token:

- **Parse error** → first an unterminated string literal, then a mistyped keyword. An open quote
  (e.g. `… = "toto`) is a common parse failure that the keyword scan cannot help; it is detected
  structurally by scanning the raw query for a quote (`'` or `"`) that is never closed, and the fix
  is the original query with the matching quote appended — accepted only if the appended-quote query
  actually parses (so a bad guess is never offered). This is tried before the keyword scan because
  an open quote makes the rest of the lexing meaningless. Otherwise the keyword case is handled by
  scanning the raw query's bareword tokens (outside quotes) and finding the first one whose closest
  match in the supported keyword list clears the threshold and is not already an exact keyword (see
  candidate decision below).
- **`resolve resource "<name>"`** → a mistyped table name; `<name>` is extracted from the message.
- **`unknown variable: '<name>'`** / **`object field access of field '<field>' ... without that field`**
  → a mistyped field; the token is extracted from the message. As a special case, when the unknown
  variable name contains a dot (e.g. `spec.annotations`) it is dotted sub-field access, not a typo:
  the suggestion converts the full dotted chain to the `->` operator (`dot-notation` kind) and
  reminds the user that sub-fields use `->`. This is checked before similarity-based field
  correction. octosql may report only the trailing segments (`labels.app` for `metadata.labels.app`),
  so the converter expands to the maximal contiguous dotted chain in the query containing the
  reported token, avoiding unrelated dotted tokens such as `notes.json` file sources.

The three are tried in pipeline order, so the first failing stage determines which single token is
corrected (precedence keyword → table → field).

- **Why over alternatives:** Walking the logical plan to find every token reference would duplicate
  octosql's resolution rules (qualifiers, aliases, map vs struct) and drift from them. Keying off
  each stage's error keeps a single source of truth.
- **Trade-off:** Couples to octosql's error/panic message strings (and, for keywords, to a raw-token
  scan since the parser error is unspecific). Mitigated by small adapter-local matchers with focused
  tests; if a message changes, exactly one matcher updates. The matchers live in the octosql adapter
  (the only package allowed to know octosql internals).

### Decision: Candidate set is the valid token set for the failing stage
Keyword candidates are the supported SQL keyword list; table candidates are the queryable resource
names; field candidates are scoped to the parent at which the unknown field was accessed — never the
whole table's field list when the typo is nested. Field resolution:

- **Top-level typo** (`unknown variable: '<name>'`, e.g. `staus`): candidates are the table's
  top-level schema field names.
- **Nested typo** (`object field access of field '<field>' ...`, e.g. `spec->contenairs`): the
  suggestion logic walks the `->` chain **before** the failing segment down the inferred schema and
  uses the SubFields of the resolved parent as the candidate set. For `spec->contenairs`, candidates
  are exactly the subfields under `spec` (`containers`, `nodeName`, `volumes`, …) — not unrelated
  top-level or `status` fields. This generalizes to arbitrary depth: for `spec->containers->imagee`
  the parent is the `containers` list element struct, so candidates are a Container's subfields
  (`image`, `name`, `ports`, …).

The schema tree (struct SubFields, and list-element SubFields for `list[index]->field` access) comes
from the schema port — the same enumeration the Tab-completion source already walks for arrow-chain
completion (`internal/adapter/shell/completion`, `arrowChainRe`). Reusing that walk keeps map-vs-struct
and list-element handling consistent.

Note that octosql evaluates a `->` chain from its base: if the base variable is itself a typo (e.g.
`spc->contenairs`), typecheck fails first with `unknown variable: 'spc'` — a **top-level** error —
so the base is corrected first against the top-level field set (`spc`→`spec`), and the deeper typo
(`contenairs`) only surfaces on the next run. The `object field access` path is therefore reached
only when every segment before the failing one is valid, so the parent walk normally succeeds; if it
nonetheless cannot resolve the reported parent against the inferred schema, no candidate set is
available and no suggestion is produced for that run.

### Decision: Similarity matching is a `spellchecker` port with a `strutil` adapter (hexagonal)
To respect the project's hexagonal architecture, similarity matching is a **port**, not a utility the
octosql adapter imports directly. The octosql engine depends only on the port interface; the concrete
similarity library lives behind an adapter, mirroring the existing `logger` port / `zap` adapter
split.

- **Port** `internal/port/spellchecker` — a domain-owned interface:
  ```go
  package spellchecker
  // SpellChecker suggests the closest valid candidate for a possibly-mistyped token.
  type SpellChecker interface {
      // ClosestMatch returns the candidate most similar to target and true when a
      // candidate clears the similarity threshold; otherwise "", false.
      ClosestMatch(target string, candidates []string) (string, bool)
  }
  ```
- **Adapter** `internal/adapter/spellchecker` — the **only** package importing
  [`github.com/adrg/strutil`](https://github.com/adrg/strutil). Its `New()` returns a
  `spellchecker.SpellChecker`. It scores candidates with `strutil.Similarity` using the
  **Jaro-Winkler** metric (`metrics.NewJaroWinkler()`, case-insensitive — rewards a shared prefix and
  handles the common identifier-typo classes), returns the highest-scoring candidate when its score
  ≥ `0.85`, and breaks ties deterministically (shortest candidate, then lexicographic). The threshold
  is deliberately conservative — a wrong suggestion is worse than none. Empirically genuine
  single-typo corrections score ≥ ~0.89 (e.g. `pdos`→`pods` 0.93, `staus`→`status` 0.96) while
  unrelated tokens score ≤ ~0.64 (e.g. `toot`→`pods` 0.50, `test`→`events` 0.64), so `0.85` sits in a
  wide empty gap between the two populations. It also
  rejects any candidate more than **30% longer** than the typo (with an absolute floor of one extra
  character, so single-character additions like `nam`→`name` still pass): without this guard a short
  typo like `toot` can score above threshold against a long unrelated token like
  `replicationcontrollers`, which is never a plausible correction. The metric, the `0.85` threshold,
  and the length bound are private adapter details, covered by the adapter's unit tests, so they can
  be retuned without touching callers.

The `SpellChecker` is **injected** into the octosql engine via its constructor
(`octosql.New(config, ds, spellchecker)`), wired in `internal/domain/commands/query` alongside the
existing DataSource wiring (and reused for the REPL path). Tests inject a fake `SpellChecker`.

- **Why over a direct util call:** keeping similarity behind a port confines the new `adrg/strutil`
  dependency to one adapter, lets the metric be swapped or faked without touching the engine, and
  matches the codebase's no-global-state, constructor-injection convention. The dependency is
  justified here per the project's dependency guardrail; it is small and pure-Go.

### Decision: Build the corrected query by whole-word token substitution on the original SQL string
The corrected query is the **original query text** with the single offending token replaced by the
suggested token (whole-word / whole-identifier match, first occurrence). Reusing the raw query text
keeps the suggestion readable and faithful to what the user typed (formatting, casing of the rest).

### Decision: Confirmation behavior depends on interactivity AND surface
The corrected query is never executed without an explicit user action, but the mechanism differs
between the one-shot CLI and the REPL.

- **One-shot CLI (TTY):** print a kind-appropriate line ending in the corrected query — e.g.
  `error: field <typo> does not exist, run this query instead ? <fixed query>` (field/table), or a
  `did you mean <kw>?` phrasing for keywords — and prompt. Default answer is **yes** (Enter runs it)
  since the corrected query is read-only. On yes, the corrected query is executed in place of the
  failed one; on no, exit 1.
- **REPL (interactive):** do **not** prompt yes/no. Print a diagnostic naming the mistyped and
  suggested tokens (`error: field <typo> does not exist, did you mean <suggestion>?`) and **pre-fill
  the corrected query into the next prompt** for editing, using the readline library's
  `WriteStdin` ("fillable stdin") so the cursor sits at the end of the pre-filled line. The user
  presses Enter to run it as-is, edits it first, or clears it — the correction runs only when the
  (possibly edited) line is submitted as a normal query. This is friendlier than a yes/no prompt:
  the suggestion is a starting point the user can refine, not a take-it-or-leave-it choice. The
  octosql engine returns the `SuggestionError` unchanged in REPL mode (`c.inREPL`); the REPL loop
  (`internal/adapter/shell/readline`) detects it via `errors.As`, prints the hint, and pre-fills.
- **Non-interactive (piped stdin / batch):** print the full suggestion line (with the corrected
  query, since there is no editable prompt) but do **not** run it. This honors "ask confirmation" —
  a guessed correction is never executed unattended.

The one-shot CLI and REPL therefore do not share a single confirm-and-rerun helper: the one-shot
path owns the yes/no prompt (in `internal/domain/commands/query`), while the REPL owns the pre-fill
(it alone holds the readline instance whose input line is being filled).

### Decision: Suggestion lives behind the SQL-engine port boundary
The detection + ranking + corrected-query construction is performed in the octosql adapter (it owns
parse, typecheck, and octosql error shapes) and surfaced via a typed result the callers can act on —
e.g. the engine returns a structured suggestion error/value carrying `{ kind, typo, suggestion,
correctedSQL }`. The one-shot command and REPL share one confirmation+re-run helper so behavior is
identical. octosql internals do not leak past the adapter.

## Risks / Trade-offs

- **[Coupling to octosql error/panic strings]** → matchers are isolated and unit-tested against the
  exact messages (`external/octosql/logical/logical.go` for fields, `database.go` `resolve resource`
  for tables); a vendored-version bump re-runs those tests.
- **[Keyword scan false positive]** mistaking an identifier/value for a keyword typo → the scan runs
  only after a parse failure, skips quoted literals, ignores exact keywords, and requires the
  similarity threshold + user confirmation; a wrong guess costs one keystroke.
- **[Wrong guess annoys the user]** → only one token changes, the full corrected query is shown
  before running, and confirmation is required.
- **[Token substitution hits the wrong occurrence]** when the same token appears more than once
  → replace only the first whole-word occurrence and rely on the user reviewing the shown query;
  acceptable because we correct one typo per attempt and the user confirms.
- **[Infinite re-suggestion loop]** if the corrected query still fails → each run corrects at most
  one token and requires fresh confirmation, so there is no automatic loop.

## Open Questions

- Confirmation default for the interactive one-shot path: `yes` (since read-only) vs `no` to mirror
  the DELETE prompt convention. **Resolved: `yes`.**
- REPL confirmation mechanism: yes/no prompt vs pre-filling the corrected query into the input line.
  **Resolved: pre-fill** — the REPL prints a diagnostic and seeds the next prompt with the corrected
  query (editable; Enter to run), which is friendlier than a binary prompt and lets the user refine
  the suggestion.
