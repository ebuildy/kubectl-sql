## 1. File split (done on branch)

- [x] 1.1 Add `schema_default.go` with `defaultSchemaProvider` returning the baseline fields
- [x] 1.2 Add `schema_openapi.go` with `openAPIInferrer`
- [x] 1.3 Add `schema_sample.go` with `sampleInferrer`
- [x] 1.4 Add `schema.go` with `strategicSchemaProvider` and `mergeSchemas`
- [x] 1.5 Remove obsolete `inferrer.go` and `inferrer_test.go`

## 2. Defaults-first merge logic

- [x] 2.1 Build `root` object field from `defaultSchemaProvider` in `strategicSchemaProvider.Provide`
- [x] 2.2 Merge OpenAPI fields into `root` via `mergeSchemas` (layer 1)
- [x] 2.3 Merge sample-object fields into `root` via `mergeSchemas` (layer 2, no longer either/or)
- [x] 2.4 Implement recursive `mergeSchemas`: append new fields, recurse on objects, prefer object form on enrichment, error only on leaf-vs-leaf conflict

## 3. Naming + bug fixes

- [x] 3.1 Rename the `schemaInferrer` method `InferFields` → `Provide` on the interface and all providers (default/openapi/sample/strategic) and call sites (`datasource.go`, tests)
- [x] 3.2 Fix `openAPITypeToFieldType` to type `$ref`/composition schemas (e.g. `metadata` → ObjectMeta) as `object` instead of `string`
- [x] 3.3 Fix `mergeSchemas` slice-reallocation bug: index by position and append new fields once at the end so `&root.SubFields[i]` pointers stay valid across appends
- [x] 3.4 Sample inferrer unions fields across a small batch (`sampleLimit`) so dynamic keys like `metadata.labels.app` are reliably discovered
- [x] 3.5 Remove the dead commented-out legacy merge block from `schema.go`
- [x] 3.6 Remove the stray `pod.json` scratch file from the repo root

## 4. Verification

- [x] 4.1 Update `TestSchema_MergeSchemas` to cover append, recurse, object-wins enrichment, and leaf-vs-leaf conflict
- [x] 4.2 Run `make lint build` (0 issues)
- [x] 4.3 Run `go test ./... -race -count=1` (all packages pass)
- [x] 4.4 Run `make e2e` and `make e2e-run-fake` (all tests pass)

## 5. List type + length() on nested fields

- [x] 5.1 Add `schema.FieldTypeList` and infer slices as a list (walk `typeOf`, OpenAPI `array`)
- [x] 5.2 Map `FieldTypeList` to octosql `TypeIDList` and build list values (JSON-string elements) in `database.go`; render lists as JSON arrays
- [x] 5.3 Fix `length()` descriptors: discriminating `TypeFn` per kind (was: every TypeFn returned ok, so string always won → counted chars / returned 0 for structs)
- [x] 5.4 Add white-box envtest test asserting `metadata.labels` is a struct and `spec.volumes` is a list
- [x] 5.5 Add e2e scenario: `length(metadata->labels)`=1, `length(spec->volumes)`=1, volumes renders as a JSON array
- [x] 5.6 Re-run `make lint build`, `go test ./... -race`, `make e2e`, `make e2e-run-fake` (all pass)
