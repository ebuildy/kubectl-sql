Feature: SQL query execution

  Scenario: Invalid SQL exits 1 and prints an error
    When I run "kubectl-sql NOT VALID SQL"
    Then the exit code is 1

  Scenario: No kubeconfig and valid SQL exits non-zero
    When I run "kubectl-sql SELECT name FROM pods"
    Then the exit code is not 0

  Scenario: SHOW TABLES with no cluster exits non-zero
    When I run "kubectl-sql --kubeconfig /nonexistent SHOW TABLES"
    Then the exit code is not 0

  Scenario: DESCRIBE TABLE with no cluster exits non-zero
    When I run "kubectl-sql --kubeconfig /nonexistent DESCRIBE TABLE pods"
    Then the exit code is not 0

  Scenario: DESCRIBE TABLE with no resource name exits 1
    When I run "kubectl-sql --kubeconfig /nonexistent DESCRIBE TABLE"
    Then the exit code is 1
