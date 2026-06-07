## ADDED Requirements

### Requirement: Kubernetes data source is defined by a domain-typed port
The Kubernetes integration SHALL be expressed as a `DataSource` interface in `internal/port/datasources/k8s`. No exported type or method signature on the port SHALL reference `k8s.io/client-go`, `k8s.io/apimachinery`, `k8s.io/kube-openapi`, or other Kubernetes libraries. Resources, rows, and schema SHALL be expressed in plain Go and domain types (e.g. `[]map[string]any`, `schema.Field`).

#### Scenario: Port signatures are library-free
- **WHEN** the `internal/port/datasources/k8s` package is compiled
- **THEN** it does not import any `k8s.io/*` package, and its interface uses only standard-library and domain types

#### Scenario: Resolve a table name to a resource
- **WHEN** the port is asked to resolve `"po"` (or `"pods"`, or the `Pod` kind)
- **THEN** it returns a canonical resource identity for pods, or an error if the name is unknown

### Requirement: Library is confined to the adapter package
All Kubernetes client libraries SHALL be imported only by the adapter package `internal/adapter/datasources/k8s` and the `cmd` composition root that wires it. No other package SHALL import `k8s.io/client-go`, `k8s.io/apimachinery`, `k8s.io/kube-openapi`, or `k8s.io/client-go/discovery`.

#### Scenario: Only the adapter imports client-go
- **WHEN** the source tree is scanned for imports of `k8s.io/client-go`, `k8s.io/apimachinery`, `k8s.io/kube-openapi`, or `k8s.io/client-go/discovery`
- **THEN** those imports appear only within `internal/adapter/datasources/k8s` and the `cmd` composition root

#### Scenario: Data source can be swapped without touching consumers
- **WHEN** the client-go adapter is replaced by another adapter satisfying the `DataSource` port
- **THEN** no package outside `internal/adapter/datasources/*` and the `cmd` wiring requires modification

### Requirement: Port exposes listing, schema, and resource discovery
The `DataSource` port SHALL provide: paginated listing of a resource's objects as `[]map[string]any`; schema inference for a resource as `[]schema.Field`; and enumeration of all queryable resources (names plus aliases) for `SHOW TABLES` and completion.

#### Scenario: Paginated list returns plain objects
- **WHEN** a consumer lists a resource through the port with a page size
- **THEN** it receives the objects as `[]map[string]any` without any client-go types crossing the boundary

#### Scenario: Schema inference returns domain fields
- **WHEN** a consumer infers a resource's schema through the port
- **THEN** it receives `[]schema.Field` (the existing field model), with server-managed metadata fields omitted as before

#### Scenario: Resource enumeration backs SHOW TABLES
- **WHEN** `SHOW TABLES` is executed
- **THEN** the table list is produced from the port's resource enumeration, identical to the current output

### Requirement: Existing query behavior is preserved
Routing queries, `SHOW TABLES`, `DESCRIBE TABLE`, REPL completion, and watch through the port SHALL produce the same results, exit codes, and output formats as before the refactor.

#### Scenario: Query output is unchanged
- **WHEN** any previously-passing query is run after the refactor
- **THEN** its output and exit code match the pre-refactor behavior (the existing integration suite passes unchanged)
