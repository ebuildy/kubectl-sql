## Why

When a query result cell renders as pretty (beautified) JSON in table output, string values
containing newlines (e.g. a ConfigMap `data` entry holding a shell script) are shown with the
JSON `\n` escape sequence instead of real line breaks. The cell collapses a multi-line script
into one unreadable line, defeating the purpose of "beautify" mode for exactly the values
(`configmaps.data`, `secrets.stringData`, etc.) that most need it.

## What Changes

- In table output, when beautify/pretty mode is enabled, struct/list/tuple/map cells render as
  YAML by default. YAML's literal block scalar style (`|`) renders string values containing
  embedded newlines as real line breaks natively, with no `\n` escape sequences.
- A JSON beautify format remains available via an internal constant
  (`beautifyFormatActive = beautifyFormatJSON`): pretty-printed JSON cells render embedded
  JSON-escaped newlines (`\n`) in string values as real line breaks, while all other JSON escaping
  (`\"`, `\\`, etc.) is left untouched.
- `--output json` and `--output csv` are unaffected and remain strictly valid JSON/CSV.
  `--disable-beauty` (compact JSON cells) is unaffected.
- When `--color-keys` is enabled:
  - YAML beautify cells (the default) have their top-level (root) mapping keys wrapped in ANSI
    cyan — e.g. `phase` in a `status` cell — matching the JSON key-coloring style. Nested keys,
    sequence-item keys, values, and literal block scalar content are never colorized.
  - JSON beautify cells continue to have all object keys colored (`ColorizeJSONKeys`) at every
    depth, computed before the newline conversion so embedded newlines cannot be mistaken for new
    JSON lines during colorization.

## Capabilities

### Modified Capabilities

- `output-renderer`: pretty-printed (beautify) struct/list/tuple/map cells in table output render
  as YAML by default (multi-line strings as literal block scalars), with a JSON beautify format
  (multi-line strings as real line breaks) available via an internal constant. `--output json`,
  `--output csv`, and `--disable-beauty` continue to render those values as valid
  single-line-escaped JSON, unchanged from before this change. When `--color-keys` is enabled,
  YAML cells color their top-level mapping keys and JSON cells color all object keys.

## Impact

- `internal/utils/utils.go` — `UnescapeJSONNewlines` (real line breaks for JSON beautify cells) and
  `ColorizeYAMLTopLevelKeys` (colors root-level YAML mapping keys), alongside the existing
  `ColorizeJSONKeys`/`AnsiCyan`/`AnsiReset` helpers.
- `internal/adapter/sql/octosql/render.go` — `renderTable` branches on `beautifyFormatActive`
  (default `beautifyFormatYAML`): YAML cells via `yaml.Marshal` + `ColorizeYAMLTopLevelKeys`; JSON
  cells via `json.MarshalIndent` + `ColorizeJSONKeys` + `UnescapeJSONNewlines`.
- New/updated unit tests in `internal/utils/utils_test.go` and
  `internal/adapter/sql/octosql/render_test.go` covering: multi-line string values in both
  beautify formats, CSV/`--output json`/`--disable-beauty` remaining unaffected, and color-key
  correctness (full-depth for JSON, top-level only for YAML, including block-scalar content).
- `gopkg.in/yaml.v3` promoted from an indirect to a direct dependency — no new module enters the
  dependency graph.
- No grammar change, no CLI flag change.
