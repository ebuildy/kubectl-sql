# Spec: SHOW TABLES

## Purpose

Defines the behavior of the `SHOW TABLES` introspection command, which lists all Kubernetes API resource types queryable via `kubectl-sql`.

---

## Requirements

### Requirement: SHOW TABLES lists all queryable Kubernetes resource types
Running `SHOW TABLES` SHALL return a table of all API resources discoverable via the cluster's discovery API, sorted alphabetically by name, with columns: `NAME` (resource plural name), `ALIASES` (comma-separated short names, empty if none), `GROUP` (API group, empty for core resources), `VERSION`.

#### Scenario: SHOW TABLES returns resource list
- **WHEN** the user runs `kubectl-sql "SHOW TABLES"`
- **THEN** the output table contains at least the core resources (`pods`, `services`, `configmaps`, `namespaces`) sorted alphabetically, and exits 0

#### Scenario: SHOW TABLES shows short name aliases
- **WHEN** the user runs `kubectl-sql "SHOW TABLES"`
- **THEN** resources with short names (e.g. `pods` → `po`, `services` → `svc`) show them in the ALIASES column

#### Scenario: SHOW TABLES includes CRDs
- **WHEN** CRDs are installed in the cluster
- **THEN** `SHOW TABLES` output includes those custom resource types

#### Scenario: SHOW TABLES respects --kubeconfig and --context flags
- **WHEN** the user runs `kubectl-sql --context other-ctx "SHOW TABLES"`
- **THEN** resources from that context's cluster are listed

---

### Requirement: SHOW TABLES is case-insensitive
The command SHALL accept `show tables`, `SHOW TABLES`, `Show Tables`, and any mixed-case variant as equivalent.

#### Scenario: Lowercase variant accepted
- **WHEN** the user runs `kubectl-sql "show tables"`
- **THEN** the command exits 0 and returns the resource table
