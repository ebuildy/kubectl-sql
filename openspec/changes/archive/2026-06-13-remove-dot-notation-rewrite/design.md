## Context

`internal/adapter/sql/octosql/rewrite.go` contains `rewriteQuery`, which today does
two things: (1) qualify bare table names with `k8s.` so octosql routes them to
`KubernetesDatabase`, and (2) — formerly — call `rewriteDottedFields`, a five-regex
pipeline that rewrote dot notation, wildcard suffixes, bracket map-key access, and
struct-then-index chains into arrow notation and `map_get`/`array_get` calls. (2) is
already disabled (`internal/adapter/sql/octosql/rewrite.go:23`). Only (1) runs.

The `->` operator is native to octosql's `sqlparser` and needs no rewriting. The
helper functions (`map_get`, `map_contains_key`, `map_values`, `keys`, `contains`,
`array_get`) are registered directly in `FunctionMap()` and are called as ordinary
SQL function calls — also no rewriting needed. So the dead code in (2), its dedicated
unit tests, a fully-commented-out test file, a commented-out test function, and
commented-out e2e scenarios are the only things referencing the old dot grammar in
code. The rest of the references are documentation (README, AGENTS.md) and two
long-lived specs describing the old/disabled behavior as if it were live.

## Goals / Non-Goals

**Goals:**
- Remove the disabled `rewriteDottedFields` pipeline and every regex it used.
- Remove tests and e2e scenarios that exist only to exercise that dead pipeline.
- Bring README.md, AGENTS.md, and `docs/grammar.ebnf` in line with the syntax that
  actually works (`->` + helper functions), removing dot-notation examples.
- Update the `k8s-datasource` and `sql-engine-port` specs so they no longer claim a
  dot-notation rewrite step exists.

**Non-Goals:**
- No change to runtime query behavior — the rewriter was already disabled, so
  deleting it changes nothing observable.
- No new SQL syntax, functions, or schema changes.
- Not addressing the broader `distinguish-map-vs-struct-fields` change or its open
  schema-size questions — this change is scoped to the dot-rewrite code path and its
  documentation footprint only.

## Decisions

- **Delete rather than re-enable.** Re-enabling `rewriteDottedFields` was
  considered and rejected: the maintainer disabled it due to edge cases, `->` and
  the helper functions already cover every case it targeted, and keeping a
  regex-based string-rewrite layer "just in case" is the complexity this change
  removes.
- **`rewriteQuery` keeps its table-qualifier rewrite.** That part is live, tested
  (indirectly, via every query that hits `k8s.<table>`), and unrelated to dot
  notation. Its doc comment is updated to stop describing a "dot/arrow rewrite"
  it no longer performs.
- **Treat the doc/spec updates as part of this change, not a follow-up.** Because
  README/AGENTS.md currently advertise dot notation as working ("auto-rewritten",
  "equivalent"), leaving them as-is after deleting the code would make the docs
  actively wrong rather than just incomplete.

## Risks / Trade-offs

- [Risk] AGENTS.md's "Common Debug Recipes" and "SQL Grammar Reference" sections use
  dot notation pervasively; rewriting every example to `->`/function form is
  mechanical but touches many lines. → Mitigation: tasks.md enumerates each
  doc/section explicitly so `/opsx:apply` can work through them without missing one.
- [Risk] `test/e2e/features/map.feature` has both live scenarios and the
  commented-out dead ones; care is needed to remove only the dead blocks. →
  Mitigation: the dead blocks are clearly marked `@TODO skip until we improve
  rewrite stuff` and fully commented out, making them unambiguous to find and
  delete.

## Open Questions

None.
