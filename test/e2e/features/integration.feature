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
