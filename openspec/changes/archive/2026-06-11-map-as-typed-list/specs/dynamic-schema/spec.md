## MODIFIED Requirements

### Requirement: Object columns use octosql TypeIDStruct
The schema model SHALL distinguish two object kinds:

- **Struct** (`FieldTypeObject`): a fixed, known schema such as `metadata`, `spec`, `status`. A struct has a bounded set of named subfields discoverable from OpenAPI. Maps that are also structs nest recursively as structs.
- **Map** (`FieldTypeMap`): an open-ended `map[string]T` such as `labels`, `annotations`. A map has a single declared value type and an unbounded, per-object set of keys.

Inference SHALL classify a field as a map when OpenAPI declares `type: object` with `additionalProperties` and no `properties` (or, for the default baseline, for the well-known fields `labels` and `annotations`); otherwise an object with `properties`/`$ref` is a struct. A field's kind, once set by the default baseline or OpenAPI, SHALL NOT be changed by a later sample layer.

A struct SHALL materialize as `octosql.TypeIDStruct` (so `->` field access works). A map SHALL materialize as `octosql.TypeIDList` whose element type is `octosql.Any` ("a typed map list"), holding that row's entries as a flat, alternating sequence `[key1, value1, key2, value2, ...]`. Each `keyN` SHALL be `octosql.String`; each `valueN` SHALL be the value's native octosql type (`Int`, `Float`, `Boolean`, or `String`), with nested objects/arrays falling back to a JSON-string element (matching how list-typed columns encode composite elements). octosql has no native map type and its Struct is a fixed positional shape, so a struct cannot represent per-row varying keys — a typed map list preserves both per-row keys and each value's native type. Slices (`FieldTypeList`) SHALL continue to be typed as `octosql.TypeIDList` with a `String` element type, distinguishing them from a map list (`Element` is `Any`) at the type level.

Map key access SHALL use bracket syntax `map['key']`, which the query rewriter lowers to `map_get(map, 'key')` (returns the key's value in its native type, or NULL if absent). `keys()`, `contains()`, `length()`, and `map_values()` SHALL operate on a map's flat key/value list (keys at even indices, values at odd indices), and JSON output SHALL render a map column as a JSON object with each value in its native JSON type (string, number, boolean).

#### Scenario: A fixed-schema field is a struct
- **WHEN** the schema is inferred for a pod's `metadata`
- **THEN** `metadata` is a struct (`FieldTypeObject`) with named subfields (`name`, `namespace`, …)

#### Scenario: An open-ended field is a map
- **WHEN** the schema is inferred for a pod's `metadata.labels`
- **THEN** `metadata.labels` is a map (`FieldTypeMap`), not a fixed-schema struct

#### Scenario: Map keys do not become top-level columns
- **WHEN** the user runs `DESCRIBE TABLE pods` or `SELECT * FROM pods`
- **THEN** `labels` and `annotations` each appear as a single column, and their per-object keys are NOT expanded into separate columns

#### Scenario: Each row keeps its own map keys
- **WHEN** two pods have different label sets and the user runs `SELECT metadata->labels FROM pods`
- **THEN** each row renders its own labels as a JSON object, with no keys dropped or invented across rows

#### Scenario: Map key access and helpers work
- **WHEN** the user runs `SELECT metadata.labels['app'], keys(metadata->labels), contains(metadata->labels, 'nginx'), length(metadata->labels) FROM pods WHERE name = 'nginx'`
- **THEN** `metadata.labels['app']` returns the key's value (NULL if absent), and `keys()`, `contains()`, and `length()` resolve against that row's flat key/value list

#### Scenario: map_get returns the value's native type
- **WHEN** a pod has an annotation whose value is an RFC3339 timestamp (e.g. `kubectl.kubernetes.io/restartedAt`)
- **THEN** `map_get(metadata->annotations, 'kubectl.kubernetes.io/restartedAt')` returns a `Time` value, not a JSON-escaped string, consistent with how other timestamp fields are typed

#### Scenario: A slice field is a list
- **WHEN** the schema is inferred for a pod with `spec.volumes`
- **THEN** `spec.volumes` is a list (`TypeIDList` with `Element` type `String`), not a struct or a map list (`Element` type `Any`)
