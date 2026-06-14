## 1. Schema model semantics

- [x] 1.1 Document in `internal/port/schema/field.go` that for a `FieldTypeList` field, `SubFields` describes the **element** schema (update the `Field` doc comment).
- [x] 1.2 Update `internal/port/schema/walk.go` doc comment for list handling (sample walk still emits childless lists — sample data has no element schema; note this explicitly so the contract is clear).

## 2. Swagger generator: resolve list element schema

- [x] 2.1 In `tools/genk8sschema/fields.go` `schemaToField`, when `classify` returns `FieldTypeList`, resolve the array `Items` schema: if it is a `$ref` to (or inline) an object, recurse via `propertiesToFields` using `depth+1` and the shared `visiting` cycle guard to populate the list field's `SubFields`.
- [x] 2.2 Leave `SubFields` nil when the element is scalar, a map, or has no resolvable object schema. Respect the existing `maxDepth` cap and ignored-field filtering for element subfields.
- [x] 2.3 Add a `refName`/items helper if needed to read `spec.Schema.Items.Schema` safely (handle nil Items / Items.Schema).
- [x] 2.4 Verify `classify` keeps open-ended map properties (`additionalProperties`, no `properties` — e.g. `metadata->labels`, `spec->nodeSelector`, Container `resources->limits`) as `FieldTypeMap` with no `SubFields` at every depth, including inside resolved list elements; never emit them as childless `FieldTypeObject`.

## 3. Live OpenAPI inferrer parity

- [x] 3.1 In `internal/adapter/datasources/k8s/schema_openapi.go` `openAPISchemaToField`, resolve array element object schemas into the list field's `SubFields`, mirroring the generator (bounded recursion, ignored-field filtering).

## 4. octosql typing

- [x] 4.1 In `internal/adapter/sql/octosql/database.go` `fieldToOctoType`, in the `FieldTypeList` branch: when `len(f.SubFields) > 0`, build a `Struct` element type from the subfields (reuse the object branch's struct-building); otherwise keep the `String` element as today. Do not touch the map branch (`Element: Any`).

## 5. octosql value materialization

- [x] 5.1 Generalize `anyToListValue` (database.go) to accept the element subfields and, for each raw element that is a `map[string]interface{}` when subfields are present, build a struct via `resolveMapAsStruct`; fall back to the current JSON-string encoding otherwise.
- [x] 5.2 Update call sites to pass element subfields: `resolveFieldValue` (list branch), `resolveStructValue` (list sub-branch), and `resolveMapAsStruct` (list sub-branch).
- [x] 5.3 Verify struct arity always matches the declared element type (missing keys → NULL) so octosql does not panic on mismatched struct shape.

## 6. Output renderer

- [x] 6.1 In `internal/adapter/sql/octosql/render.go` `valueToNativeTyped`, add a `List<Struct>` branch that decodes each element via `valueToNativeTyped(elem, elementStructType)` so element field names resolve; keep JSON-string decoding for `List<String>`.
- [x] 6.2 Confirm `rendersAsJSON` keeps `List<Struct>` cells on the existing beautify path so table output renders pretty **YAML** by default (and JSON when the internal constant is `beautifyFormatJSON`).
- [x] 6.3 Confirm `--output json` and `--output csv` render `List<Struct>` as arrays of objects.

## 7. Regenerate embedded snapshot

- [x] 7.1 Run `make generate` to refresh `internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go` with list element subfields; verify it is deterministic (run twice → byte-identical).

## 8. Unit tests — swagger schema loader / generator (nested schema focus)

- [x] 8.1 Generator: `spec->containers` for `pods` resolves to `FieldTypeList` with element `SubFields` including `name`, `image`, `ports` (and `ports` itself a nested list).
- [x] 8.2 Generator: scalar-element arrays (e.g. a `[]string` command/args) remain `FieldTypeList` with no `SubFields`.
- [x] 8.3 Generator: deeply nested element subfields resolve through `$ref` chains (e.g. `spec->containers[].env`, `spec->volumes[].*`) and terminate at the depth cap.
- [x] 8.4 Generator: self-referential element schemas terminate (cycle guard) without infinite recursion.
- [x] 8.5 Generator: `make generate` output remains reproducible/byte-identical with element subfields enabled.
- [x] 8.6 Loader (`schema_swagger_loader_test.go`): `swaggerSchemaProvider.Provide` for `pods` returns `spec->containers` as a list whose `SubFields` carry the Container element schema, after the guaranteed-fields prefix.
- [x] 8.7 Loader: round-trip gob encode/decode preserves list element `SubFields` (nested several levels deep).
- [x] 8.8 Loader: resources/fields with scalar-element lists round-trip with nil `SubFields` (no spurious children).
- [x] 8.9 Generator: open-ended map properties (`metadata->labels`, `metadata->annotations`, `spec->nodeSelector`, and a Container `resources->limits` nested inside the `spec->containers` list element) resolve to `FieldTypeMap` with no `SubFields`, not childless objects.

## 9. Unit tests — typing, values, rendering, end-to-end

- [x] 9.1 `fieldToOctoType`: `FieldTypeList` with element subfields → `List<Struct{...}>`; without subfields → `List<String>`; map stays `List<Any>`.
- [x] 9.2 Value materialization: a list-of-object field resolves to an octosql `List` of `Struct` values with positional fields matching subfields (missing keys → NULL).
- [x] 9.3 Renderer: `List<Struct>` cell renders as a YAML sequence of named-key mappings (default) and as a JSON array of objects under `--output json`; scalar lists unchanged.
- [x] 9.4 Query (engine/integration test): `SELECT name, spec->containers[0]->name FROM pods LIMIT 2` type-checks, executes, and projects the first container name.
- [x] 9.5 Query: out-of-range index (`spec->containers[99]->name`) yields NULL, exit 0.
- [x] 9.6 Query: `WHERE spec->containers[0]->image = '...'` filters rows.
- [x] 9.7 Regression: scalar-element list indexing (`array_get` / `[i]`) returns the JSON-encoded element as before.

## 10. Validation

- [x] 10.1 `make lint build` passes.
- [x] 10.2 `make test` passes (`go test ./... -race -count=1`).
- [x] 10.3 Manually verify the acceptance query `kubectl sql "SELECT name, spec->containers[0]->name FROM po LIMIT 2"` against envtest/a cluster (or document the integration test that covers it).
