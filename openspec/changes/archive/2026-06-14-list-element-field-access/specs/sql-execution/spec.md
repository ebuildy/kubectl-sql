## ADDED Requirements

### Requirement: List element fields are accessible by index and field path
The query engine SHALL allow accessing a single element of a list-typed column that carries a known element object schema (its element is a struct) by integer index, and then accessing that element's fields with the `->` operator, i.e. `list[index]->field` (and deeper, e.g. `list[index]->sub->field`). Indexing SHALL return the element struct, or NULL when the index is out of range, and field access on a possibly-NULL element SHALL yield NULL rather than erroring. Element struct values SHALL materialize from the raw resource object so the projected values match the underlying data. List columns without a known element schema SHALL keep their existing scalar/JSON-string element behavior (`array_get` / indexing returns a JSON-encoded element).

#### Scenario: Selecting a nested list element field returns the value
- **WHEN** the user runs `kubectl-sql "SELECT name, spec->containers[0]->name FROM pods LIMIT 2"`
- **THEN** the query type-checks and executes, and the second column shows each pod's first
  container name (not a JSON blob or an error), then exits 0

#### Scenario: Out-of-range index yields NULL
- **WHEN** the user selects `spec->containers[99]->name` for a pod with fewer than 100 containers
- **THEN** the cell resolves to NULL and the query exits 0 without error

#### Scenario: List element field usable in WHERE
- **WHEN** the user runs `kubectl-sql "SELECT name FROM pods WHERE spec->containers[0]->image = 'nginx'"`
- **THEN** the query executes and returns pods whose first container image is `nginx`

#### Scenario: Scalar-element list access is unchanged
- **WHEN** the user indexes a list column that has no known element object schema (e.g. a `[]string`)
- **THEN** indexing returns the JSON-encoded element as before, with no regression
