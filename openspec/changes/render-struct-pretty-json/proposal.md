# Proposal: render-struct-pretty-json

## Why

When a selected column is a struct (e.g. `SELECT status FROM pods`), table and CSV output fall back to octosql's positional `Value.String()`, printing values without their field names (`{ Running, 10.0.0.1, ... }`). The result is unreadable and loses the key information users need.

## What Changes

- Table output renders struct-typed cells as pretty-printed (indented) JSON with field names resolved from the schema type.
- List-, tuple-, and map-typed cells get the same treatment: pretty JSON arrays/objects in table output (map columns decode to real JSON objects, as `--output json` already does), compact in CSV.
- CSV output renders struct/list/tuple/map cells as compact single-line JSON (valid inside a quoted CSV field).
- Cell rendering gains access to the schema field type (`valueToString` currently receives only the value, but struct field names live in `octosql.Type`).
- New CLI flag `--disable-beauty` (default `false`): when set, struct cells render as compact single-line JSON in table output too, with no coloring.
- JSON keys in pretty-printed struct cells are colored (keys only, not values) when stdout is a TTY and `--no-color` is not set.
- JSON output format (`--output json`) is unchanged (it already resolves struct field names via `valueToNativeTyped`; machine-readable output is never colored).

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `output-renderer`: struct-typed values in table and CSV output SHALL render as JSON (pretty in table, compact in CSV) instead of octosql's positional string form; pretty JSON keys are colored on TTY; `--disable-beauty` turns off pretty-printing and coloring.

## Impact

- `internal/adapter/sql/octosql/render.go` — `renderTable`, `renderCSV`, `valueToString` (signature change to accept the schema type or a typed wrapper), key colorization.
- `cmd/root.go` — new `--disable-beauty` flag; `internal/port/api/api.go` and `internal/port/sql/engine.go` — carry the setting (and color enablement) to the renderer.
- Existing unit tests on rendering; new tests for struct cell rendering in table and CSV, flag behavior, and color gating.
- No new dependencies, no grammar change. One new CLI flag (`--disable-beauty`).
