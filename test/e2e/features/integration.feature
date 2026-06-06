Feature: SQL queries against envtest cluster

  Scenario: Cross-namespace pod listing returns results
    When I run kubectl-sql "SELECT name, namespace FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length >= 30"

  Scenario: Namespace-scoped pod query returns subset
    Given I pick a random fixture namespace
    When I run kubectl-sql with namespace query "SELECT name FROM pods WHERE namespace = '<fixture-namespace>'" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length >= 3 and length <= 5"

  Scenario: LIMIT caps results
    When I run kubectl-sql "SELECT name FROM pods LIMIT 5" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length <= 5"

  Scenario: Deployments listing returns results
    When I run kubectl-sql "SELECT name, namespace FROM deployments" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length >= 10"

  Scenario: ConfigMaps listing returns results
    When I run kubectl-sql "SELECT name, namespace FROM configmaps" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length >= 10"

  Scenario: SHOW TABLES lists queryable resources
    When I run kubectl-sql "SHOW TABLES" against the envtest cluster
    Then the exit code is 0
    And the output contains "pods"

  Scenario: JSON output format returns valid JSON array
    When I run kubectl-sql --output "json" with query "SELECT name FROM pods LIMIT 3" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length <= 3 and .[0] | has(\"pods.name\")"

  Scenario: CSV output format returns CSV with header
    When I run kubectl-sql --output "csv" with query "SELECT name FROM pods LIMIT 3" against the envtest cluster
    Then the exit code is 0
    And the output contains "name"

  # Dynamic schema — SELECT *
  Scenario: SELECT * from pods includes status column
    When I run kubectl-sql "SELECT * FROM pods LIMIT 1" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0] | has(\"pods.status\")"

  Scenario: SELECT * from pods includes metadata column
    When I run kubectl-sql "SELECT * FROM pods LIMIT 1" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0] | has(\"pods.metadata\")"

  Scenario: SELECT * from deployments includes spec column
    When I run kubectl-sql "SELECT * FROM deployments LIMIT 1" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0] | has(\"deployments.spec\")"

  Scenario: SELECT * from configmaps includes name column
    When I run kubectl-sql "SELECT * FROM configmaps LIMIT 1" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0] | has(\"configmaps.name\")"

  # Dynamic schema — SELECT specific real fields
  Scenario: SELECT status from pods returns status values
    When I run kubectl-sql "SELECT name, namespace, status FROM pods LIMIT 1" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0] | has(\"pods.status\")"

  # Dynamic schema — WHERE on real field
  Scenario: WHERE on namespace field works with dynamic schema
    Given I pick a random fixture namespace
    When I run kubectl-sql with namespace query "SELECT name FROM pods WHERE namespace = '<fixture-namespace>'" against the envtest cluster
    Then the exit code is 0

  # DESCRIBE TABLE — known resources
  Scenario: DESCRIBE TABLE pods lists name column
    When I run kubectl-sql "DESCRIBE TABLE pods" against the envtest cluster
    Then the exit code is 0
    And the output contains "name"

  Scenario: DESCRIBE TABLE pods lists namespace column
    When I run kubectl-sql "DESCRIBE TABLE pods" against the envtest cluster
    Then the exit code is 0
    And the output contains "namespace"

  Scenario: DESCRIBE TABLE pods lists status column
    When I run kubectl-sql "DESCRIBE TABLE pods" against the envtest cluster
    Then the exit code is 0
    And the output contains "status"

  Scenario: DESCRIBE TABLE pods omits server-managed metadata fields
    When I run kubectl-sql "DESCRIBE TABLE pods" against the envtest cluster
    Then the exit code is 0
    And the output does not contain "managedFields"
    And the output does not contain "resourceVersion"

  Scenario: DESCRIBE TABLE configmaps lists name column
    When I run kubectl-sql "DESCRIBE TABLE configmaps" against the envtest cluster
    Then the exit code is 0
    And the output contains "name"

  Scenario: DESCRIBE TABLE deployments lists spec column
    When I run kubectl-sql "DESCRIBE TABLE deployments" against the envtest cluster
    Then the exit code is 0
    And the output contains "spec"

  # DESCRIBE TABLE — case insensitivity
  Scenario: describe table lowercase is accepted
    When I run kubectl-sql "describe table pods" against the envtest cluster
    Then the exit code is 0
    And the output contains "name"

  # Error cases
  Scenario: SELECT from unknown resource exits 1
    When I run kubectl-sql "SELECT name FROM doesnotexist" against the envtest cluster
    Then the exit code is not 0

  Scenario: DESCRIBE TABLE with no resource name exits 1
    When I run kubectl-sql "DESCRIBE TABLE" against the envtest cluster
    Then the exit code is not 0

  # GROUP BY and aggregates
  Scenario: GROUP BY namespace counts pods per namespace
    When I run kubectl-sql "SELECT namespace, COUNT(*) FROM pods GROUP BY namespace" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length >= 10"

  Scenario: COUNT(*) returns a number greater than 30
    When I run kubectl-sql "SELECT COUNT(*) FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0] | to_entries | .[0].value >= 30"

  Scenario: DESCRIBE TABLE shows metadata column
    When I run kubectl-sql "DESCRIBE TABLE pods" against the envtest cluster
    Then the exit code is 0
    And the output contains "metadata"

  Scenario: SELECT metadata column contains object data
    When I run kubectl-sql "SELECT name, metadata FROM pods LIMIT 1" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0][\"pods.metadata\"] | has(\"resourceVersion\")"

  Scenario: SELECT metadata->labels->app returns nginx (arrow notation)
    When I run kubectl-sql "SELECT DISTINCT metadata->labels->app FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "[.[].\"pods.metadata->labels->app\"] | any(. == \"nginx\")"

  Scenario: SELECT metadata.labels.app returns nginx (dot notation rewritten to arrow)
    When I run kubectl-sql "SELECT DISTINCT metadata.labels.app FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "[.[].\"pods.metadata->labels->app\"] | any(. == \"nginx\")"

  Scenario: SELECT metadata->labels returns labels struct
    When I run kubectl-sql "SELECT DISTINCT metadata->labels FROM pods LIMIT 1" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0][\"pods.metadata->labels\"] | has(\"app\")"

  Scenario: SELECT metadata.labels.* expands to labels struct
    When I run kubectl-sql "SELECT DISTINCT metadata.labels.* FROM pods LIMIT 1" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0][\"pods.metadata->labels\"] | has(\"app\")"

  Scenario: WHERE on nested struct field filters correctly
    When I run kubectl-sql "SELECT name FROM pods WHERE metadata->labels->app = 'nginx'" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length > 0"

  # Nginx pod with ConfigMap volume
  Scenario: spec.volumes[0].configMap shows nginx-config
    When I run kubectl-sql --namespace "nginx-test" with query "SELECT name, namespace, spec.volumes[0].configMap FROM pods WHERE name = 'nginx'" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0][\"pods.spec_volumes_0_configMap\"].name == \"nginx-config\""

  Scenario: WHERE on name with LIKE pattern
    When I run kubectl-sql "SELECT name FROM pods WHERE name LIKE '%amber%' OR name LIKE '%bold%' OR name LIKE '%crisp%'" against the envtest cluster
    Then the exit code is 0

  Scenario: ORDER BY name sorts results
    When I run kubectl-sql "SELECT name FROM pods ORDER BY name LIMIT 5" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length <= 5"

  Scenario: SELECT with multiple columns and LIMIT
    When I run kubectl-sql "SELECT name, namespace, status FROM pods LIMIT 3" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length <= 3 and .[0] | has(\"pods.namespace\")"

  Scenario: --namespace flag scopes COUNT(*) to main namespace
    When I run kubectl-sql --namespace "main" with query "SELECT COUNT(*) FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ ".[0] | to_entries | .[0].value == 2"

  # Watch mode
  Scenario: --watch re-executes query and shows table output
    When I run kubectl-sql --watch "SELECT name, namespace FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length >= 30"

  Scenario: --watch works with ORDER BY and LIMIT
    When I run kubectl-sql --watch "SELECT name FROM pods ORDER BY name LIMIT 5" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length <= 5"

  # REPL batch mode (piped stdin, no positional query)
  Scenario: Piped query runs in batch mode and exits 0
    When I pipe "SELECT name FROM pods LIMIT 1" to kubectl-sql against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "length <= 1 and .[0] | has(\"pods.name\")"
