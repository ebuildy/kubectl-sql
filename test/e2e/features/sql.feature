Feature: SQL query execution

  Scenario: Invalid SQL exits 1 and prints an error
    When I run "kubectl-sql NOT VALID SQL"
    Then the exit code is 1

  Scenario: No kubeconfig and valid SQL exits non-zero
    When I run "kubectl-sql SELECT name FROM pods"
    Then the exit code is not 0
