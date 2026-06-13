## 1. Remove the dead rewrite pipeline

- [x] 1.1 In `internal/adapter/sql/octosql/rewrite.go`, delete `rewriteDottedFields` and the regexes `dottedWildcardRe`, `dottedFieldRe`, `arrowIndexRe`, `mapKeyAccessRe`, `stringLiteralRe`, and the commented-out call site / `@TODO` comment in `rewriteQuery`. Keep the table-qualifier rewrite (`FROM <resource>` → `FROM k8s.<resource>`) and update `rewriteQuery`'s doc comment to describe only that.
- [x] 1.2 Delete `internal/adapter/sql/octosql/rewrite_test.go` (tests only `rewriteDottedFields`).
- [x] 1.3 Delete `internal/adapter/sql/octosql/array_index_query_test.go` (fully commented-out scaffold).
- [x] 1.4 In `internal/adapter/sql/octosql/map_query_test.go`, remove the commented-out `TestMapField_AccessKeysContains` function and its doc comment.

## 2. Fix stale comments

- [x] 2.1 In `internal/adapter/sql/octosql/functions.go`, update the `mapGetFunction` and `arrayGetFunction` doc comments to stop referencing `rewriteDottedFields`; describe `map_get`/`array_get` as plain SQL function calls used alongside `->`.
- [x] 2.2 In `internal/adapter/sql/octosql/engine.go`, update the package doc comment to drop "dot/arrow rewrite" and describe the pipeline as table-qualifier rewrite, parse, plan, typecheck, optimize, execute, render.

## 3. Remove dead e2e scenarios

- [x] 3.1 In `test/e2e/features/map.feature`, remove the commented-out `@TODO skip until we improve rewrite stuff` scenarios (dot + bracket map-key access on `metadata.labels['app']`, and `metadata.labels.*`).

## 4. Update documentation

- [x] 4.1 In `README.md`, remove the "or dot notation (auto-rewritten)" clause from the Features list; rewrite the array-indexing bullet to use `array_get(spec->volumes, 0)` form instead of `spec.volumes[0].configMap`.
- [x] 4.2 In `README.md`, update the REPL example query from `status.phase` to `status->phase`.
- [x] 4.3 In `README.md` "Nested fields" section, remove the "Dot notation is automatically rewritten" sentence and the dot-notation example block; rewrite the array-index example using `array_get`.
- [x] 4.4 In `AGENTS.md`, update the top-of-file example queries and the "SQL Grammar Reference (summary)" section to use `->` and helper functions (`map_get`, `array_get`, `keys`, `contains`) instead of dot notation (`status.phase`, `.metadata.labels['...']`); remove the "Support dot notation" bullet.
- [x] 4.5 In `AGENTS.md` "Common Debug Recipes" section, rewrite each example query that uses dot notation (`status.phase`, `status.reason`, `status.replicas`, `status.availableReplicas`, `status.conditions[?...]`, `.status.containerStatuses[0]...`) to the equivalent `->`/helper-function form.
- [x] 4.6 In `docs/grammar.ebnf`, note (or fill in) that nested access is `->` plus helper functions only — no dot-notation production.

## 5. Verify

- [x] 5.1 Run `make lint build` and `go test ./... -race -count=1`.
- [x] 5.2 Run `make e2e` (or the relevant e2e target) to confirm `map.feature` still passes after scenario removal.
