## ADDED Requirements

### Requirement: SHOW TABLES lists all queryable Kubernetes resource types
Running `SHOW TABLES` SHALL return a table of all API resources discoverable via the cluster's REST mapper, with columns: `name` (resource plural name), `group` (API group, empty for core), `version`, `namespaced` (true/false).

#### Scenario: SHOW TABLES returns resource list
- **WHEN** the user runs `kubectl-sql "SHOW TABLES"`
- **THEN** the output table contains at least the core resources (`pods`, `services`, `configmaps`, `namespaces`) and exits 0

#### Scenario: SHOW TABLES includes CRDs
- **WHEN** CRDs are installed in the cluster
- **THEN** `SHOW TABLES` output includes those custom resource types

#### Scenario: SHOW TABLES respects --kubeconfig and --context flags
- **WHEN** the user runs `kubectl-sql --context other-ctx "SHOW TABLES"`
- **THEN** resources from that context's cluster are listed

### Requirement: SHOW TABLES is case-insensitive
The parser SHALL accept `show tables`, `SHOW TABLES`, and `Show Tables` as equivalent.

#### Scenario: Lowercase variant accepted
- **WHEN** the user runs `kubectl-sql "show tables"`
- **THEN** the command exits 0 and returns the resource table
