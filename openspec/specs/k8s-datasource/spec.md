# Spec: Kubernetes Datasource

## Purpose

Defines how the `kubectl-sql` datasource layer resolves Kubernetes resource kinds, fetches objects from the cluster, maps them to query rows, and applies namespace scoping. These requirements govern the interaction between the SQL execution engine and the Kubernetes API.

---

## Requirements

### Requirement: Resource kind is resolved via REST mapper
The datasource SHALL resolve the FROM clause table name to a Kubernetes GVR (GroupVersionResource) using the REST mapper. Short names, plural forms, and full kind names SHALL all be accepted.

#### Scenario: Plural form resolves correctly
- **WHEN** the FROM clause is `pods`
- **THEN** the datasource resolves to `core/v1/pods` and lists pods

#### Scenario: Plural form resolves correctly 2
- **WHEN** the FROM clause is `pod`
- **THEN** the datasource resolves to `core/v1/pods` and lists pods

#### Scenario: Short name resolves correctly
- **WHEN** the FROM clause is `po`
- **THEN** the datasource resolves to `core/v1/pods` and lists pods

#### Scenario: Unknown resource returns an error
- **WHEN** the FROM clause is `doesnotexist`
- **THEN** the datasource returns an error and the query exits 1

---

### Requirement: Resources are fetched with pagination
The datasource SHALL paginate LIST calls using the `--page-size` flag value (default 500) to avoid loading the entire cluster into memory.

#### Scenario: Page size is respected
- **WHEN** `--page-size 10` is set and there are 25 pods
- **THEN** the datasource makes at least 3 LIST calls and returns all 25 pods as rows

---

### Requirement: Each resource object is exposed as a row
Each Kubernetes object returned by the LIST call SHALL be exposed as an octosql row. The row columns SHALL be derived from the OpenAPI schema or a sample object at query planning time. Guaranteed columns are `name` and `namespace`. Additional columns correspond to top-level keys (e.g. `status`, `spec`, `metadata`). Fields absent on a given object resolve to NULL.

#### Scenario: Field access on a known field
- **WHEN** the query selects `status.phase` from pods
- **THEN** the value matches the pod's `.status.phase` field in the cluster

#### Scenario: Missing field returns NULL
- **WHEN** the query selects a field path that does not exist on a resource
- **THEN** the value for that field is NULL (not an error)

#### Scenario: All top-level fields are accessible
- **WHEN** the user runs `SELECT * FROM pods`
- **THEN** columns include at minimum `name`, `namespace`, `status`, `spec`, `metadata`

---

### Requirement: Nested objects are exposed as octosql structs
Map values (e.g. `metadata`, `status`) SHALL be represented as `octosql.TypeIDStruct` with named fields inferred from the sample and/or OpenAPI schema. Nested struct access uses the `->` operator natively supported by octosql. Slices SHALL remain serialized as JSON strings.

#### Scenario: Top-level struct field access
- **WHEN** the user runs `SELECT metadata->labels FROM pods LIMIT 1` with `--output json`
- **THEN** the output contains `"app"` as a key in the struct value

#### Scenario: Deep struct field access
- **WHEN** the user runs `SELECT metadata->labels->app FROM pods`
- **THEN** the output contains `nginx`

---

### Requirement: No flattened underscore alias columns
There SHALL be no synthetic `metadata_labels`, `metadata_labels_app` style alias columns. Nested field access is expressed via `->` operator only. The dot-notation rewriter SHALL convert `metadata.labels.app` → `metadata->labels->app` before parsing.

#### Scenario: Dot notation is rewritten to arrow notation
- **WHEN** the user runs `SELECT metadata.labels.app FROM pods`
- **THEN** the query is rewritten to `metadata->labels->app` and returns `nginx`

---

### Requirement: Namespace scoping is applied
When `--namespace` is provided, the datasource SHALL restrict LIST calls to that namespace. When omitted, the datasource SHALL list across all namespaces.

#### Scenario: Namespace flag restricts list scope
- **WHEN** `--namespace kube-system` is set
- **THEN** the LIST call uses namespace `kube-system` and cluster-scoped resources are still accessible
