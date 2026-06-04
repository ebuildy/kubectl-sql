## ADDED Requirements

### Requirement: envtest API server is started and stopped per suite
The test suite SHALL start an envtest API server in `TestMain`, seed fixture data, run all scenarios, then stop the server regardless of test outcome.

#### Scenario: Server starts successfully
- **WHEN** `make e2e-run-fake` is run with `setup-envtest` installed
- **THEN** the envtest API server starts and a valid kubeconfig temp file is written

#### Scenario: Server stops after tests
- **WHEN** the godog suite finishes (pass or fail)
- **THEN** `envtest.Environment.Stop()` is called and the server process exits

### Requirement: Fixture data is seeded before scenarios run
The test suite SHALL create exactly 10 namespaces with random names, each containing 3–5 Pods (phase=Running), 1–2 Deployments, and 1–2 ConfigMaps before any scenario executes.

#### Scenario: Namespaces are created
- **WHEN** fixture seeding completes
- **THEN** exactly 10 namespaces (excluding system namespaces) exist in the envtest cluster

#### Scenario: Pods are in Running phase
- **WHEN** fixture seeding completes
- **THEN** all seeded pods have `status.phase = Running`

### Requirement: SELECT query returns seeded pods
Running `kubectl-sql "SELECT name, namespace FROM pods"` against the envtest cluster SHALL return all seeded pods across all namespaces.

#### Scenario: Cross-namespace pod listing
- **WHEN** `kubectl-sql "SELECT name, namespace FROM pods"` is run against the envtest cluster
- **THEN** the output table contains at least 30 rows (10 namespaces × min 3 pods each) and exits 0

### Requirement: WHERE clause filters by namespace
Running a query with `WHERE namespace = '<name>'` SHALL return only pods from that namespace.

#### Scenario: Namespace filter returns subset
- **WHEN** `kubectl-sql "SELECT name FROM pods WHERE namespace = '<one-fixture-namespace>'"` is run
- **THEN** the output contains between 3 and 5 rows (the seeded count for that namespace)

### Requirement: LIMIT clause caps results from envtest cluster
The LIMIT clause SHALL work correctly against real envtest-served resources.

#### Scenario: LIMIT 5 on pods
- **WHEN** `kubectl-sql "SELECT name FROM pods LIMIT 5"` is run against the envtest cluster
- **THEN** at most 5 rows appear in the output and the command exits 0

### Requirement: Deployments and ConfigMaps are queryable
Seeded Deployments and ConfigMaps SHALL be accessible via SQL queries.

#### Scenario: Deployments listing
- **WHEN** `kubectl-sql "SELECT name, namespace FROM deployments"` is run
- **THEN** the output contains at least 10 rows (10 namespaces × min 1 deployment) and exits 0

#### Scenario: ConfigMaps listing
- **WHEN** `kubectl-sql "SELECT name, namespace FROM configmaps"` is run
- **THEN** the output contains at least 10 rows and exits 0
