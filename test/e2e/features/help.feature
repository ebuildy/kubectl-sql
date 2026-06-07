Feature: CLI help

  Scenario: Help flag prints usage and exits 0
    When I run "kubectl-sql --help"
    Then the exit code is 0
    And the output contains "kubectl-sql"
