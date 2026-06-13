Feature: JSON Lines file datasource

  Scenario: SELECT * from a local JSON Lines file returns its rows
    When I run kubectl-sql "SELECT * FROM test/fixtures/notes.jsonl" against the envtest cluster
    Then the exit code is 0
    And the output has between 2 and 2 rows
    And the output contains "nginx-1"
    And the output contains "nginx-2"
    And the output contains "check logs"
