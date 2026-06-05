Feature: SQL queries against envtest cluster

  Scenario: Cross-namespace pod listing returns results
    When I run kubectl-sql "SELECT name, namespace FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output has at least 30 rows

  Scenario: Namespace-scoped pod query returns subset
    Given I pick a random fixture namespace
    When I run kubectl-sql with namespace query "SELECT name FROM pods WHERE namespace = '<fixture-namespace>'" against the envtest cluster
    Then the exit code is 0
    And the output has between 3 and 5 rows

  Scenario: LIMIT caps results
    When I run kubectl-sql "SELECT name FROM pods LIMIT 5" against the envtest cluster
    Then the exit code is 0
    And the output has at most 5 rows

  Scenario: Deployments listing returns results
    When I run kubectl-sql "SELECT name, namespace FROM deployments" against the envtest cluster
    Then the exit code is 0
    And the output has at least 10 rows

  Scenario: ConfigMaps listing returns results
    When I run kubectl-sql "SELECT name, namespace FROM configmaps" against the envtest cluster
    Then the exit code is 0
    And the output has at least 10 rows

  Scenario: SHOW TABLES lists queryable resources
    When I run kubectl-sql "SHOW TABLES" against the envtest cluster
    Then the exit code is 0
    And the output contains "pods"

  Scenario: JSON output format returns valid JSON array
    When I run kubectl-sql --output "json" with query "SELECT name FROM pods LIMIT 3" against the envtest cluster
    Then the exit code is 0
    And the output contains "["
    And the output contains "name"

  Scenario: CSV output format returns CSV with header
    When I run kubectl-sql --output "csv" with query "SELECT name FROM pods LIMIT 3" against the envtest cluster
    Then the exit code is 0
    And the output contains "name"

  Scenario: --namespace flag scopes COUNT(*) to a single namespace
    When I run kubectl-sql --namespace "main" with query "SELECT COUNT(*) FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output contains "2"
