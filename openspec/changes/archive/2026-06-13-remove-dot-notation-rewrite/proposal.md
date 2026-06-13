## Why

The query-string rewriter that converts dot notation (`metadata.labels.app`,
`metadata.labels['app']`, `spec.volumes[0]`) into arrow notation and helper-function
calls (`metadata->labels->app`, `map_get(...)`, `array_get(...)`) is a multi-pass
regex pipeline (`rewriteDottedFields` plus five regexes for wildcards, array
indexes, map-key brackets, and string-literal protection) that the maintainer
already disabled (`internal/adapter/sql/octosql/rewrite.go:23`) because of edge
cases. It is dead code: kept alive only by unit tests that call it directly and
by commented-out e2e scenarios marked "skip until we improve rewrite stuff".

The `->` operator (native to octosql's parser) plus `map_get`, `map_contains_key`,
`map_values`, `keys`, `contains`, and `array_get` already cover every case the
rewriter targeted, with no string-rewriting required. Carrying both a "dot"
grammar and an "arrow + functions" grammar doubles the documented SQL surface
(README, AGENTS.md examples, the `sql-engine-port` and `k8s-datasource` specs all
describe dot notation as supported/equivalent) for a form that doesn't actually
work today, misleading users and inflating the spec surface for no behavioral
value.

## What Changes

- Delete `rewriteDottedFields` and its regexes (`dottedWildcardRe`,
  `dottedFieldRe`, `arrowIndexRe`, `mapKeyAccessRe`, `stringLiteralRe`) and the
  commented-out call site in `internal/adapter/sql/octosql/rewrite.go`.
  `rewriteQuery` keeps only its table-qualifier rewrite (`FROM pods` →
  `FROM k8s.pods`).
- Delete `internal/adapter/sql/octosql/rewrite_test.go` (tests only the removed
  function) and `internal/adapter/sql/octosql/array_index_query_test.go` (fully
  commented-out scaffold).
- Remove the commented-out `TestMapField_AccessKeysContains` from
  `internal/adapter/sql/octosql/map_query_test.go`.
- Remove the commented-out "@TODO skip until we improve rewrite stuff" scenarios
  from `test/e2e/features/map.feature` (dot + bracket map-key access,
  `metadata.labels.*`).
- Update stale comments referencing `rewriteDottedFields` in `functions.go`
  (`mapGetFunction`, `arrayGetFunction`) and the package doc in `engine.go`
  (drop the "dot/arrow rewrite" framing).
- **BREAKING (docs only — no runtime behavior change)**: Update README.md,
  AGENTS.md, and the `docs/grammar.ebnf` placeholder to document `->` and the
  helper functions (`map_get`, `map_contains_key`, `map_values`, `keys`,
  `contains`, `array_get`) as the only supported nested-field/array/map syntax.
  Remove all dot-notation examples (`status.phase`, `metadata.labels['app']`,
  `spec.volumes[0].configMap`, etc.).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `k8s-datasource`: Remove the "No flattened underscore alias columns" requirement's
  dot-notation-rewriter clause and its scenario; nested access is `->` (and helper
  functions) only, with no rewrite step.
- `sql-engine-port`: Drop "dot/arrow query rewriting" from the pipeline description
  (the adapter performs only table-qualifier rewriting) and remove the "Dot and
  arrow field paths still work" scenario.

## Impact

- Code: `internal/adapter/sql/octosql/rewrite.go`, `rewrite_test.go`,
  `array_index_query_test.go`, `map_query_test.go`, `functions.go` (comments),
  `engine.go` (package doc).
- Tests: `test/e2e/features/map.feature` (remove dead scenarios).
- Docs: `README.md`, `AGENTS.md`, `docs/grammar.ebnf`.
- Behavior: none — the rewriter was already disabled, so no query that works
  today stops working. This only removes dead code and corrects documentation
  that currently describes non-functional syntax.
