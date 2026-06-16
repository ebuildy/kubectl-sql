## 1. Spellchecker port and adapter

- [x] 1.1 Add the port `internal/port/spellchecker` with interface `SpellChecker { ClosestMatch(target string, candidates []string) (string, bool) }` (domain-owned, no library imports)
- [x] 1.2 Add `github.com/adrg/strutil` to `go.mod` (`go get github.com/adrg/strutil`) and run `go mod tidy`
- [x] 1.3 Implement the adapter `internal/adapter/spellchecker` (the only package importing `strutil`): `New() spellchecker.SpellChecker`, scoring candidates with `strutil.Similarity` + `metrics.NewJaroWinkler()` (case-insensitive), returning the highest-scoring candidate when its score ≥ 0.85 (conservative — prefer no suggestion over a wrong one), rejecting candidates more than 30% longer than the typo (absolute floor of one extra char), ties broken deterministically (shortest, then lexicographic); unit test threshold boundary, ties, the no-match case, low-confidence junk rejection, and the over-long-candidate rejection (`toot`↛`replicationcontrollers`)

## 2. Inject the spellchecker and detect the mistyped token per stage

- [x] 2.1 Add the injected `spellchecker.SpellChecker` parameter to `octosql.New(config, ds, spellchecker)`, store it on the engine, and update the wiring in `internal/domain/commands/query` (and the REPL path) to construct `spellcheckerAdapter.New()` and pass it
- [x] 2.2 Define a structured suggestion type (e.g. `{ Kind, Typo, Suggestion, CorrectedSQL string }`, Kind ∈ keyword/table/field) and a typed error/value the engine can return so callers can act on it
- [x] 2.3 Parse-failure detection: (a) unterminated string literal — scan the raw query for an unclosed `'`/`"` (honouring backslash escapes) and propose the query with the matching quote appended, accepted only if the appended-quote query re-parses (kind `syntax`, tried first); (b) mistyped keyword — scan the raw query's bareword tokens (skipping quoted literals) and pick the first token that is not already an exact keyword whose `SpellChecker.ClosestMatch` against the supported keyword list (reuse completion `sqlKeywords`) succeeds; unit test the missing-quote case (single+double), `SLECT`/`FORM`, balanced-quote no-op, and the quoted-literal exclusion
- [x] 2.4 Table detection: add a matcher for `resolve resource "<name>"` from a recovered typecheck error and extract the resource token; unit test the match and no-match cases
- [x] 2.5 Field detection: add matchers for `unknown variable: '<name>'` and `object field access of field '<field>' ... without that field` and extract the offending field token; treat an unknown-variable name containing a dot as dotted sub-field access (convert the full dotted chain to `->`, `dot-notation` kind, with a reminder to use `->`) checked before similarity matching; unit test both shapes plus the dot-notation conversion (single and multi-level)

## 3. Build the suggestion in the engine

- [x] 3.1 In `engine.Execute`, try the stages in pipeline order — parse failure → keyword candidates (supported keyword list); table-resolution failure → table candidates (`ds.Resources()` names); field typecheck failure → field candidates scoped to the failing position via the schema port (top-level field names for `unknown variable`; for a nested `->` typo walk the chain preceding the failing segment, including list-element struct SubFields for `list[index]->field`, reusing the completion arrow-chain walk; no suggestion if the parent can't be resolved)
- [x] 3.2 Rank candidates with the injected `SpellChecker.ClosestMatch`; if a match is found, build `CorrectedSQL` by whole-word / whole-identifier, first-occurrence substitution of the typo token in the original query text; return the structured suggestion
- [x] 3.3 When no candidate is within threshold or the failure is not a single-mistyped-token case, return the original error unchanged
- [x] 3.4 Unit test (with a fake `SpellChecker`): unterminated quote (single+double), keyword typo, table typo, top-level field typo, nested field typo, cross-stage precedence (`SLECT name FROM pdos` corrects keyword first), two-field typo (only first corrected), no-match passthrough, rest-of-query preserved verbatim

## 4. Confirmation + re-run (shared)

- [x] 4.1 Add a one-shot confirmation helper that, given a suggestion, prints the kind-appropriate line ending in `run this query instead ? <corrected query>`, prompts on a TTY (default yes), and re-runs the corrected query on confirmation; on non-TTY it prints the line and does not run
- [x] 4.2 Wire the one-shot path (`internal/domain/commands/query`) to the helper: interactive prompt + run on yes; non-interactive prints suggestion and exits 1; no-match keeps original error + exit 1
- [x] 4.3 Wire the REPL (`internal/adapter/shell/readline`): in REPL mode the engine's `SuggestionError` is surfaced unchanged (no prompt); the interactive loop prints the diagnostic hint (`did you mean <suggestion>?`) and pre-fills the corrected query into the next prompt via readline `WriteStdin` for editing; batch mode prints the full suggestion line and continues without running; no-match prints the original error; the REPL never exits

## 5. Tests & docs

- [x] 5.1 Add engine/adapter tests covering the `query-typo-suggestion` scenarios (keyword/table/field detection, ranking, precedence, one-at-a-time, confirmation gating)
- [x] 5.2 Add coverage for the `sql-execution` one-shot scenarios (interactive run-on-confirm incl. table typo, non-interactive print-only, no-match passthrough)
- [x] 5.3 Add coverage for the `sql-repl` scenarios (REPL mode surfaces the structured suggestion for field and keyword typos without running; pre-fill helper writes the corrected query and prints the hint without repeating the query; batch mode continues without running; no-match returns the raw error — REPL never exits)
- [x] 5.4 Run `make lint build test`; ensure clean
