## ADDED Requirements

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

### Requirement: Resources are fetched with pagination
The datasource SHALL paginate LIST calls using the `--page-size` flag value (default 500) to avoid loading the entire cluster into memory.

#### Scenario: Page size is respected
- **WHEN** `--page-size 10` is set and there are 25 pods
- **THEN** the datasource makes at least 3 LIST calls and returns all 25 pods as rows

### Requirement: Each resource object is exposed as a row
Each Kubernetes object returned by the LIST call SHALL be exposed as an octosql row where each field path maps to the corresponding value in the object's unstructured JSON.

#### Scenario: Field access on a known field
- **WHEN** the query selects `status.phase` from pods
- **THEN** the value matches the pod's `.status.phase` field in the cluster

#### Scenario: Missing field returns NULL
- **WHEN** the query selects a field path that does not exist on a resource
- **THEN** the value for that field is NULL (not an error)

### Requirement: Namespace scoping is applied
When `--namespace` is provided, the datasource SHALL restrict LIST calls to that namespace. When omitted, the datasource SHALL list across all namespaces.

#### Scenario: Namespace flag restricts list scope
- **WHEN** `--namespace kube-system` is set
- **THEN** the LIST call uses namespace `kube-system` and cluster-scoped resources are still accessible
