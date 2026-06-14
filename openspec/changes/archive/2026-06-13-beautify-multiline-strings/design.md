## Context

`internal/adapter/sql/octosql/render.go` renders struct/list/tuple/map cells in table output as
pretty-printed (indented) JSON when beautify is enabled (`pretty=true`, the `render-struct-pretty-json`
change). `json.MarshalIndent` escapes embedded newlines in string values as the two-character
sequence `\n`, so a ConfigMap `data` entry holding a multi-line shell script collapses into one
unreadable line inside the cell.

While scoping the fix, the user asked whether YAML would be a better fit for these cells (YAML's
literal block scalar style (`|`) renders multi-line strings with real line breaks natively) and
asked for both a JSON fix and a YAML alternative, switchable via an internal Go constant so they
can compare the two before settling on a default.

## Goals / Non-Goals

**Goals:**
- JSON beautify cells (the current/default behavior): embedded `\n` escapes in string values
  render as real line breaks; all other JSON escaping (`\"`, `\\`, `\t`, `\uXXXX`, ...) is
  untouched.
- Add a YAML beautify cell renderer that produces the same data (via the existing
  `valueToNativeTyped` conversion) as YAML, where multi-line strings naturally use literal block
  scalar style (`|`).
- Both formats apply only to table output with `pretty=true` (struct/list/tuple/map cells).
  `--output json`, `--output csv`, and `--disable-beauty` (`pretty=false`) are unaffected and stay
  byte-identical to today (always JSON, `\n` escaped).
- The active beautify format (JSON or YAML) is controlled by a single internal Go constant in
  `internal/adapter/sql/octosql/render.go`, defaulting to YAML (decided after a side-by-side
  comparison).
- When `--color-keys` is enabled, YAML beautify cells color their top-level (root) mapping keys
  cyan, matching `ColorizeJSONKeys`'s coloring style for JSON cells.

**Non-Goals:**
- No new CLI flag in this change. Promoting the constant to a `--beauty-format` flag is future
  work if JSON is ever needed as the default again.
- No `--output yaml` overall format (a separate, larger feature; the `-o` flag's `yaml` mention is
  pre-existing and out of scope here).
- Full-depth YAML key coloring (nested keys, sequence-item keys) — out of scope for v1; see
  Decision 5.
- No change to `renderCSV` or `renderJSON` (the `--output json`/`csv` code paths).

## Decisions

1. **Introduce a `beautifyFormat` type with `beautifyFormatJSON` / `beautifyFormatYAML` constants,
   and a single `beautifyFormatActive` package variable in `render.go`.** Switching format means
   editing this variable and rebuilding — acceptable for an experimentation toggle.
   `valueToStringTyped` branches on it only when `pretty == true` and the cell `rendersAsJSON`;
   `pretty == false` (CSV, `--disable-beauty`) always uses the existing compact JSON path
   regardless of the constant.
   - *Alternative*: a CLI flag now — rejected; the user explicitly wants a const to compare both
     before committing to a default, and a flag would need its own spec/UX decision.
   - **Resolved**: after the side-by-side comparison, `beautifyFormatActive` defaults to
     `beautifyFormatYAML` — YAML is now the default beautify cell format. `beautifyFormatJSON`
     remains available by editing the constant, fully covered by tests (item 4 below).

2. **YAML encoding via `gopkg.in/yaml.v3`, reusing `valueToNativeTyped`.** The same native
   `map[string]interface{}` / slice / scalar tree already built for JSON output is passed to
   `yaml.Marshal`. `yaml.v3` automatically renders "nice" multi-line strings (no trailing
   whitespace, no special leading characters) using literal block style (`|`), which is exactly
   the desired real-EOL rendering. `gopkg.in/yaml.v3` is already present in `go.sum` as an
   indirect dependency (pulled in transitively); this change promotes it to a direct import in
   `go.mod` — no new module enters the dependency graph.
   - *Alternative*: `sigs.k8s.io/yaml` (round-trips through `encoding/json`) — rejected; it
     produces double-quoted scalars with `\n` escapes for multi-line strings, the same problem
     this change fixes for JSON.
   - The YAML document's trailing `\n` (always added by `yaml.Marshal`) is trimmed before the
     string is placed in a table cell.

3. **JSON fix via a string-literal-aware byte scanner, `utils.UnescapeJSONNewlines`.** Walks the
   marshaled JSON tracking whether the scanner is inside a string literal; when inside a string and
   the next two bytes are the escape sequence `\n` (backslash, `n`), emit a single real newline
   byte (`0x0A`) instead; any other `\`-prefixed escape (`\"`, `\\`, `\t`, `\uXXXX`, ...) is copied
   through verbatim (2 bytes, or for `\u`, the `\u` pair — the following 4 hex digits are plain
   string characters and need no special handling). This avoids corrupting a literal
   backslash-then-`n` in the original data, which JSON encodes as `\\n` (three bytes) — the scanner
   only ever consumes the 2-byte `\n` escape as a unit.
   - *Alternative*: a regex replace of the raw bytes — rejected; a regex can't reliably
     distinguish `\n` (newline escape) from the second half of `\\n` (escaped backslash followed by
     a literal `n`) without the same string-aware state the scanner already tracks.

4. **Order of operations for JSON mode in `renderTable`**: `valueToStringTyped` (marshal) →
   `utils.ColorizeJSONKeys` (if `colorKeys`) → `utils.UnescapeJSONNewlines`. Coloring runs first,
   on valid single-line-per-key JSON, so its line-anchored key regex (`^\s*"key":`) cannot be
   confused by real newlines introduced by multi-line string content. Newline conversion runs
   last, as the final cosmetic step before the cell is appended to the table.

5. **YAML key coloring is restricted to top-level (column-0) mapping keys —
   `utils.ColorizeYAMLTopLevelKeys`.** Now that YAML is the default beautify format, ColorKeys
   should color YAML keys the same way it colors JSON keys. Unlike JSON (where every object key
   sits on its own `"key":`-prefixed line regardless of nesting, so a single line-anchored regex
   safely colors keys at every depth), YAML's literal block scalar content is real, unescaped text
   indented under its key — a line-anchored `key:`-style regex applied at any indentation level
   risks colorizing script content that merely *looks* like `key: value` (e.g. a shell comment
   `# note: see below`). Restricting the regex to column 0 (`^[^\s:#-][^:\n]*?:` or a quoted key)
   sidesteps this entirely: YAML requires block scalar content to be indented more than its key, so
   a column-0 line can only ever be a genuine root-level mapping key (or a `-` sequence marker,
   explicitly excluded).
   - *Consequence*: nested keys (e.g. `ready` under `conditions:`) and sequence-item keys (e.g.
     `name` in `- name: c1`) are not colored. This is a narrower scope than JSON's full-depth
     coloring, but safe and covers the most common case (a struct cell's immediate fields, e.g.
     `status.phase`).
   - *Alternative*: color keys at every indentation level (matching JSON) — rejected for v1; would
     require YAML-aware parsing (or tracking block-scalar indentation state) to avoid the
     block-scalar false-positive above. Left as a follow-up if full-depth coloring is wanted.

## Risks / Trade-offs

- [`yaml.v3`'s literal-block heuristic may not trigger for every string (e.g. trailing spaces,
  strings starting with `#`/`-`/quotes), falling back to a quoted scalar with `\n` escapes] →
  acceptable for v1; covered by a unit test using a representative multi-line ConfigMap value
  (`#!/bin/sh\nset -eu\n...`), which is the motivating case and does trigger literal style.
- [Two beautify code paths to maintain (JSON, YAML)] → each is small (~20-30 lines); the constant
  makes the active path explicit, and both are covered by unit tests so the inactive path doesn't
  silently break.
- [YAML key coloring is top-level only, unlike JSON's full-depth coloring] → acceptable for v1
  (Decision 5); covers the common case (struct cell's immediate fields) safely. Full-depth
  coloring is a follow-up if needed.
- [`gopkg.in/yaml.v3` becomes a direct dependency] → no new module added to the build graph
  (already transitively present); satisfies AGENTS.md's dependency-justification rule via this
  design doc.

## Open Questions

- Should YAML key coloring be extended to nested/sequence-item keys (matching JSON's full-depth
  coloring)? Would require YAML-aware indentation tracking to avoid colorizing block-scalar
  content that looks like `key: value` (see Decision 5). Left as a follow-up.

## TODO

- Pick a Go library to pretty-print/colorize JSON and YAML (full-depth key coloring, syntax
  highlighting), or write one, to replace the hand-rolled regex colorizers
  (`ColorizeJSONKeys`, `ColorizeYAMLTopLevelKeys`) and their depth/indentation limitations.
