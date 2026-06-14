Feature: DELETE statement against envtest cluster

  Scenario: DELETE with --yes removes all matched pods
    Given I seed a namespace with 5 pods
    When I run a DELETE "DELETE /* force */ pods" with --yes in that namespace
    Then the exit code is 0
    And the output contains "deleted: 5"
    And that namespace has 0 pods

  Scenario: DELETE with LIMIT only removes the capped number
    Given I seed a namespace with 5 pods
    When I run a DELETE "DELETE /* force */ FROM pods LIMIT 2" with --yes in that namespace
    Then the exit code is 0
    And the output contains "deleted: 2"
    And that namespace has 3 pods

  Scenario: DELETE with a WHERE filter deletes only matching pods
    Given I seed a namespace with 4 pods
    When I run a DELETE "DELETE /* force */ pod WHERE status->phase = 'Pending'" with --yes in that namespace
    Then the exit code is 0
    And the output contains "deleted: 4"
    And that namespace has 0 pods

  Scenario: DELETE matching nothing is a no-op and exits 0
    Given I seed a namespace with 3 pods
    When I run a DELETE "DELETE pod WHERE name = 'no-such-pod'" with --yes in that namespace
    Then the exit code is 0
    And the output contains "nothing matched"
    And that namespace has 3 pods

  Scenario: Non-interactive DELETE without --yes is refused and deletes nothing
    Given I seed a namespace with 3 pods
    When I run a DELETE "DELETE pods" without --yes in that namespace
    Then the exit code is 1
    And the output contains "--yes"
    And that namespace has 3 pods

  Scenario: --dry-run previews the plan and deletes nothing
    Given I seed a namespace with 3 pods
    When I run a DELETE "DELETE /* force */ pods" with --dry-run in that namespace
    Then the exit code is 0
    And the output contains "kubectl delete pods"
    And the output contains "--force"
    And that namespace has 3 pods

  Scenario: DELETE combined with --watch is rejected and deletes nothing
    Given I seed a namespace with 2 pods
    When I run a DELETE "DELETE pods" with --watch in that namespace
    Then the exit code is 1
    And that namespace has 2 pods
