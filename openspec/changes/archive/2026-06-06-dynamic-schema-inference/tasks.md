## 1. internal/schema package — port and shared types

- [x] 1.1 `FieldType` constants and `Field` struct
- [x] 1.2 Shared `walkObject`
- [x] 1.3 `typeOf`
- [x] 1.4 Return nil when obj is nil or empty
- [x] 1.5 `SchemaInferrer` interface and types in `port.go`
- [x] 1.6 `SubFields []Field` in `Field` struct
- [x] 1.7 Remove flattened alias emission from `walkObject` — top-level fields + SubFields only, no `metadata_labels` etc.

## 2. SampleInferrer

- [x] 2.1–2.3 `SampleInferrer` implemented

## 3. OpenAPIInferrer

- [x] 3.1–3.4 `OpenAPIInferrer` implemented
- [x] 3.5 Remove alias emission loop from `OpenAPIInferrer.InferFields` — aliases are no longer needed

## 4. CompositeInferrer

- [x] 4.1 `CompositeInferrer` struct
- [x] 4.2 Falls back on empty primary
- [x] 4.3 Remove alias-appending loop from `CompositeInferrer.InferFields` — merge SubFields only, no alias columns

## 5. Dot-notation rewriter in cmd/root.go

- [x] 5.1 Old rewriter: `metadata.labels.app` → `metadata_labels_app` (to be replaced)
- [x] 5.2 New rewriter: `metadata.labels.app` → `metadata->labels->app`
- [x] 5.3 `metadata.labels.*` → `metadata->labels` (wildcard → parent struct)
- [x] 5.4 `k8s.pods` table qualifiers remain unchanged
- [x] 5.5 Remove `dottedWildcardRe` and `dottedFieldRe` regexes; replace with arrow rewriter

## 6. Executor

- [x] 6.1–6.3 Scalar field production correct
- [x] 6.4 `NewKubernetesDatabase` accepts `SchemaInferrer`
- [x] 6.5 `GetTable` calls `inferrer.InferFields`
- [x] 6.6 `fieldToOctoType` builds `TypeIDStruct` from `SubFields`
- [x] 6.7 `Run()` produces `octosql.NewStruct` for object fields
- [x] 6.8 `Materialize` uses pruned `sch.Fields` for row ordering

## 7. Wiring

- [x] 7.1–7.4 `CompositeInferrer` wired in `cmd/root.go`

## 8. Unit Tests

- [x] 8.1–8.4 Existing schema tests
- [x] 8.5 `TestWalkObject_NestedMap`
- [x] 8.6 `TestWalkObject_Slice`
- [x] 8.7 `TestCompositeInferrer_UsesPrimaryWhenAvailable`
- [x] 8.8 `TestCompositeInferrer_FallsBackOnEmpty`
- [x] 8.9 Update `TestWalkObject_NestedMap` — assert no alias fields emitted
- [x] 8.10 Add `TestRewriteDottedFields_ArrowNotation`: `metadata.labels.app` → `metadata->labels->app`
- [x] 8.11 Add `TestRewriteDottedFields_Wildcard`: `metadata.labels.*` → `metadata->labels`

## 9. e2e scenarios

- [x] 9.1–9.14 All existing passing
- [x] 9.15 `SELECT metadata->labels->app FROM pods` exits 0 and output contains `nginx`
- [x] 9.16 `SELECT metadata.labels.app FROM pods` (dot notation) exits 0 and output contains `nginx`
- [x] 9.17 `SELECT metadata->labels FROM pods LIMIT 1 --output json` — output contains `"app"` as a key
- [x] 9.18 `SELECT name FROM pods WHERE metadata->labels->app = 'nginx'` exits 0
- [x] 9.19 Remove or update old `metadata_labels` / `metadata_labels_app` scenarios

## 10. Verification

- [x] 10.1 `go build ./...` exits 0
- [x] 10.2 `make test` exits 0
- [x] 10.3 `make lint` exits 0
- [x] 10.4 `make e2e-run-fake` — all scenarios pass
