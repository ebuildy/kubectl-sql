# Tasks: render-struct-pretty-json

## 1. Renderer changes

- [x] 1.1 Add `valueToStringTyped(v octosql.Value, t octosql.Type, pretty bool) string` in `internal/adapter/sql/octosql/render.go`: for struct-typed values, convert via `valueToNativeTyped` and marshal with `json.MarshalIndent` (pretty) or `json.Marshal` (compact), falling back to `v.String()` on marshal error; delegate all other types to `valueToString`
- [x] 1.2 Pass `opts.Schema.Fields` into `renderTable` and use `valueToStringTyped(v, field.Type, true)` for cells
- [x] 1.3 Pass `opts.Schema.Fields` into `renderCSV` and use `valueToStringTyped(v, field.Type, false)` for cells
- [x] 1.4 Add `colorizeJSONKeys(s string) string` helper that wraps keys (lines matching `^\s*"…":`) in ANSI cyan; apply to pretty struct cells only when color is enabled

## 2. Flag and config plumbing

- [x] 2.1 Add `--disable-beauty` flag (default false) in `cmd/root.go` and `DisableBeauty bool` on `api.Config`
- [x] 2.2 Forward the setting through `port/sql.Config` (next to `NoColor`) into render `Options` (`Pretty`, `ColorKeys` fields); gate `ColorKeys` on TTY && !NoColor && !DisableBeauty
- [x] 2.3 Document the flag in README flags table and `cmd/root.go` help text

## 3. Tests

- [x] 3.1 Unit test: struct cell in table output renders as indented JSON with field names (including a nested struct field)
- [x] 3.2 Unit test: struct cell in CSV output is compact single-line JSON and the output round-trips through `encoding/csv` reader as one record per row
- [x] 3.3 Unit test: scalar-only rows render byte-identical to previous behavior in table and CSV
- [x] 3.4 Unit test: `--disable-beauty` produces compact uncolored JSON in table cells; default stays pretty
- [x] 3.5 Unit test: `colorizeJSONKeys` colors keys only (positive) and never colors values containing `":` look-alikes (negative); `ColorKeys=false` output has no ANSI codes
- [x] 3.6 Verify `--output json` behavior is unchanged and CSV/JSON output never contains ANSI escapes (existing tests still pass)

## 4. List/tuple/map beautification

- [x] 4.1 Extend `valueToNative` with a `TypeIDTuple` branch (elements via `valueToNative`)
- [x] 4.2 Extend `valueToStringTyped` and the table color gating to JSON-render list, tuple, and map-list cells (helper `rendersAsJSON`), reusing `valueToNativeTyped` so map columns decode to objects
- [x] 4.3 Unit tests: list column renders as pretty JSON array (decoded elements) in table and compact in CSV; map-list column renders as JSON object; tuple renders as JSON array

## 5. Verification

- [x] 5.1 Run `make lint build` and `make test` — all clean
- [x] 5.2 Manual check against envtest/e2e fixtures: `SELECT name, status FROM pods` shows pretty JSON with colored keys on a TTY; verify table column alignment is not broken by ANSI codes; update any e2e snapshots that asserted the old positional struct form
