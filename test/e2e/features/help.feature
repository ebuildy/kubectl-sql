Feature: CLI help

  Scenario: Help flag prints usage and exits 0
    When I run "kubectl-sql --help"
    Then the exit code is 0
    And the output contains "kubectl-sql"

  Scenario: No arguments prints usage and exits 0
    When I run "kubectl-sql"
    Then the exit code is 0
    And the output contains "kubectl-sql"

  Scenario: Help flag still works after SQL argument support added
    When I run "kubectl-sql --help"
    Then the exit code is 0
    And the output contains "query"
