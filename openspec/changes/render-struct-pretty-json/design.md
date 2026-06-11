# Design: render-struct-pretty-json

## Context

`internal/adapter/sql/octosql/render.go` renders result rows in three formats. JSON output already converts struct values to named maps via `valueToNativeTyped(v, t)`, because octosql stores struct field *values* on `octosql.Value.Struct` while field *names* live on the schema's `octosql.Type.Struct.Fields`. Table and CSV cells, however, go through `valueToString(v)`, which has no type information — struct values fall to the `default:` branch and print octosql's positional `v.String()` form (field names lost).

`renderTable` and `renderCSV` already receive the row values but not the schema fields; `Render` has `opts.Schema.Fields` available and already passes them to `renderJSON`.

## Goals / Non-Goals

**Goals:**
- Struct-typed cells in table output render as pretty-printed (2-space indented) JSON with field names.
- List, tuple, and map-list cells render as JSON too (arrays / decoded objects), same pretty/compact split; `valueToNative` learns tuples (previously fell back to positional `v.String()`).
- Struct-typed cells in CSV output render as compact single-line JSON, so the encoded CSV stays one record per row.
- Reuse the existing `valueToNativeTyped` conversion — one struct→map code path.
- `--disable-beauty` flag opts out: compact single-line JSON in table cells, no coloring.
- JSON keys (only) in pretty table cells are ANSI-colored when stdout is a TTY and `--no-color` is unset.

**Non-Goals:**
- No change to `--output json` (already correct; machine-readable output is never colored).
- No change to scalar rendering.
- No coloring of JSON values, CSV output, or `--output json`.
- No cell truncation policy change.
- No REPL `\set`-style runtime toggle — the setting is a CLI flag, consistent with all other options.

## Decisions

1. **Pass schema fields into `renderTable`/`renderCSV`** (same signature shape as `renderJSON`), and introduce `valueToStringTyped(v octosql.Value, t octosql.Type, pretty bool) string` used for cells. It delegates to the existing `valueToString` for non-struct values, keeping scalar behavior byte-identical.
   - *Alternative considered*: changing `valueToString` itself to take a type — rejected because several call sites (list elements, fallbacks) have no type available; a wrapper keeps the diff small.

2. **Convert via `valueToNativeTyped` then `json.MarshalIndent` / `json.Marshal`.** Reuses the proven struct→named-map logic instead of writing a second walker.
   - *Alternative considered*: hand-building a string — rejected, duplicates logic and risks divergence from JSON output.

3. **Pretty in table, compact in CSV.** Multi-line cells are supported by `tablewriter` and are what "readable" means in a terminal; CSV consumers expect one line per record, and compact JSON inside a quoted field is standard (the `encoding/csv` writer handles quoting/escaping automatically).

4. **On marshal error, fall back to `v.String()`.** Rendering must never fail a query that executed successfully.

5. **`--disable-beauty` as a CLI flag carried through existing config structs.** Add the flag in `cmd/root.go`, a `DisableBeauty bool` on `api.Config`, and forward it to `sql.Config` (next to the existing `NoColor`) so it reaches render `Options` as `Pretty bool`. This follows the exact path `--no-color` already takes — no new plumbing pattern.
   - *Alternative considered*: a REPL `\set disable-beauty` meta-command — rejected for v1; no settings registry exists in the REPL and every other option is a flag. Can be layered on later without spec change to the flag.

6. **Color JSON keys with raw ANSI escapes, keys only.** After marshaling, colorize `"key":` tokens (cyan) via a small regex/scanner pass over the pretty JSON string. Enabled only when `stdout` is a TTY **and** `NoColor` is false **and** beauty is not disabled. CSV and `--output json` are never colored.
   - *Alternative considered*: a JSON-coloring library (e.g. `tidwall/pretty`) — rejected; one-color keys-only is ~15 lines, and AGENTS.md requires justifying every new dependency.
   - *Note*: coloring must happen after `encoding/json` marshaling (escapes inside strings make naive regex on values risky; keys-only at line starts of indented output is unambiguous: `^\s*"(...)":`).

## Risks / Trade-offs

- [Wide structs make tall table rows] → acceptable; users selecting a whole struct asked for its content. They can select sub-fields for compact output.
- [Downstream scripts parsing table output by line] → table format is human-oriented; machine consumers should use `--output json|csv`, which stays line-stable.
- [REPL shares this render path] → behavior change is intentional and consistent; covered by existing e2e snapshots if any (update fixtures as needed).
- [ANSI escapes can break `tablewriter` column-width math] → verify alignment with colored cells; if widths are wrong, colorize after table layout or disable color for that cell. `--no-color` and `--disable-beauty` are the escape hatches.
- [Non-TTY pipes must stay clean] → color is gated on TTY detection (same check as `internal/domain/commands/query/command.go`), so `kubectl sql ... | grep` sees no escape codes.
