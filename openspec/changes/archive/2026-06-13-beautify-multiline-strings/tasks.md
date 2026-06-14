## 1. JSON beautify: real line breaks for multi-line strings

- [x] 1.1 Add `UnescapeJSONNewlines(s string) string` to `internal/utils/utils.go`: a
  string-literal-aware byte scanner that converts the 2-byte JSON escape `\n` inside string
  literals into a real `\n` (0x0A) byte, copying all other characters (including other `\`-escapes
  such as `\"`, `\\`, `\t`, `\uXXXX`) through unchanged
- [x] 1.2 Add unit tests in `internal/utils/utils_test.go`: a multi-line script value (e.g.
  `"#!/bin/sh\nset -eu\nrm -rf \"$VOL_DIR\""`) converts embedded `\n` to real newlines while
  leaving `\"` and `\\` escaped; a string containing an escaped backslash followed by literal `n`
  (JSON `\\n`) is left untouched (not mistaken for a newline escape); strings with no `\n` are
  unchanged

## 2. Beautify format constant and YAML cell renderer

- [x] 2.1 In `internal/adapter/sql/octosql/render.go`, add a `beautifyFormat` type with
  `beautifyFormatJSON` / `beautifyFormatYAML` constants and a package constant
  `beautifyFormatActive = beautifyFormatJSON`
- [x] 2.2 Promote `gopkg.in/yaml.v3` from an indirect to a direct dependency (`go get
  gopkg.in/yaml.v3`, `go mod tidy`)
- [x] 2.3 In `valueToStringTyped`, when `pretty && rendersAsJSON(...)`, branch on
  `beautifyFormatActive`:
  - `beautifyFormatJSON`: existing `json.MarshalIndent` path (unchanged)
  - `beautifyFormatYAML`: `yaml.Marshal(native)` on the same `valueToNativeTyped` result, trimming
    the trailing newline `yaml.Marshal` always appends; fall back to `v.String()` on marshal error,
    same as the JSON path
- [x] 2.4 Leave the `pretty == false` path (CSV, `--disable-beauty`) untouched: always compact JSON
  via `json.Marshal`, regardless of `beautifyFormatActive`

## 3. Wire newline conversion and coloring order in renderTable

- [x] 3.1 In `renderTable`, for cells where `pretty && rendersAsJSON(...) &&
  beautifyFormatActive == beautifyFormatJSON`: apply `utils.ColorizeJSONKeys` first (if
  `colorKeys`), then `utils.UnescapeJSONNewlines` last
- [x] 3.2 For `beautifyFormatActive == beautifyFormatYAML`, skip `ColorizeJSONKeys` and
  `UnescapeJSONNewlines` entirely (YAML cells render as-is, no key coloring)

## 4. Tests

- [x] 4.1 Unit test (JSON mode, default): a struct/map cell containing a string with `\n` (e.g. a
  ConfigMap-like `data` map with a `teardown` script) renders in table output with real line
  breaks and `\"`/`\\` still escaped
- [x] 4.2 Unit test: `--output json` and `--output csv` for the same data keep `\n` escaped and
  remain valid JSON / single-line CSV records (existing round-trip assertions still pass)
- [x] 4.3 Unit test: `--disable-beauty` for the same data renders compact single-line JSON with
  `\n` still escaped
- [x] 4.4 Unit test: with `colorKeys=true` and a multi-line string value, ANSI key coloring is
  still applied correctly to the real JSON keys (not to any text resembling a key inside the
  multi-line value)
- [x] 4.5 Add a YAML-mode test (`beautifyFormatActive` changed from `const` to `var` so tests can
  temporarily override it): a struct cell renders as YAML, and a multi-line string value renders
  using literal block scalar style (`|`) with real line breaks
- [x] 4.6 Verify scalar-only rows and existing struct/list/tuple pretty-JSON tests
  (`TestRenderTableStructPretty`, `TestRenderTableListPretty`, etc.) still pass unchanged with
  `beautifyFormatActive == beautifyFormatJSON`

## 5. Verification

- [x] 5.1 Run `make lint build` and `make test` — all clean
- [x] 5.2 Manually run `kubectl-sql "SELECT name, data FROM configmaps"` against a ConfigMap with a
  multi-line `data` value (e.g. the `teardown` script from the proposal) with table output and
  beautify enabled; confirm the script renders across real lines in JSON mode
- [x] 5.3 Manually flip `beautifyFormatActive` to `beautifyFormatYAML`, rebuild, and re-run the same
  query to compare the YAML rendering; revert to JSON before committing unless the user decides to
  keep YAML as the default

## 6. YAML default and top-level key coloring

- [x] 6.1 Keep `beautifyFormatActive = beautifyFormatYAML` as the new default beautify cell format
  (decided after the side-by-side comparison in task 5.3); `beautifyFormatJSON` remains available
  by editing the variable
- [x] 6.2 Add `ColorizeYAMLTopLevelKeys(s string) string` to `internal/utils/utils.go`: a
  regex-based colorizer that wraps column-0 (top-level/root) mapping keys of indented YAML in ANSI
  cyan, leaving nested map keys, sequence-item keys, values, and literal block scalar content
  (always indented relative to its key) uncolored
- [x] 6.3 Add unit tests in `internal/utils/utils_test.go`: a struct-shaped YAML document colors
  only its root-level keys (`phase`, `conditions`), not the nested key (`ready`); a block scalar
  value whose content contains a line that looks like `key: value` is not colorized; a
  sequence-rooted document (`- name: c1`) is left unchanged
- [x] 6.4 In `renderTable`, branch on `beautifyFormatActive`: for `beautifyFormatYAML` cells, apply
  `utils.ColorizeYAMLTopLevelKeys` when `colorKeys` is enabled (no `UnescapeJSONNewlines` step, as
  before)
- [x] 6.5 Add render tests: YAML mode with `ColorKeys: true` colors the root-level struct key
  (`phase`) but not the nested key (`ready`) or values (`TestRenderTableYAMLColorKeys`); a
  multi-line block-scalar cell (`teardown`) colors only the `teardown` key, not its script content
  (`TestRenderTableYAMLColorKeysWithMultilineString`)
- [x] 6.6 Fix pre-existing JSON-specific tests broken by the default switching from JSON to YAML:
  `TestRenderTableStructPretty`, `TestRenderTableStructColorKeys`, `TestRenderTableListPretty`,
  `TestRenderTableMapMultilineStringRealNewlines`, `TestRenderTableColorKeysWithMultilineString` now
  explicitly set `beautifyFormatActive = beautifyFormatJSON` (with `defer` restore), since they
  assert JSON-specific output (quoted keys, `\n` unescaping, full-depth `ColorizeJSONKeys`) that is
  no longer the default

## 7. Final verification

- [x] 7.1 Run `make lint build` and `make test` — all clean
