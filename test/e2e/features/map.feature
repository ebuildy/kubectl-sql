Feature: SQL queries using map type and functions against envtest cluster

  Scenario: play with map type and functions
    When I run kubectl-sql "SELECT metadata->name AS name, keys(metadata->labels) AS lkeys, map_get(metadata->labels, 'app') AS app FROM po WHERE map_get(metadata->labels, 'app') = 'nginx'" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "all(.[]; .app == \"nginx\")"
    And the output produces JQ "any(.[]; .name == \"nginx\")"
    And the output produces JQ "all(.[]; .lkeys | contains([\"app\"]))"

  Scenario: map_contains_key filters pods by label presence
    When I run kubectl-sql "SELECT metadata->name AS name, map_contains_key(metadata->labels, 'app') AS has_app FROM po WHERE map_contains_key(metadata->labels, 'app')" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "all(.[]; .has_app == true)"
    And the output produces JQ "any(.[]; .name == \"nginx\")"

  Scenario: map_values returns the label values
    When I run kubectl-sql "SELECT metadata->name AS name, map_values(metadata->labels) AS vals FROM po WHERE map_get(metadata->labels, 'app') = 'nginx'" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "all(.[]; .vals | contains([\"nginx\"]))"
    And the output produces JQ "any(.[]; .name == \"nginx\")"

Scenario: use map_get in group by query
  When I run kubectl-sql "SELECT map_get(metadata->labels, 'app') AS app, count(*) AS n FROM po GROUP BY map_get(metadata->labels, 'app') ORDER BY app" against the envtest cluster
  Then the exit code is 0
  And the output produces JQ ".[0].app == \"apache\""
  And the output produces JQ ".[0].n >= 3"
  And the output produces JQ ".[1].app == \"cassandra\""
  And the output produces JQ ".[1].n >= 3"

Scenario: use map_get with config maps
  When I run kubectl-sql "SELECT map_get(data, 'key1') AS val, count(*) AS n FROM cm GROUP BY map_get(data, 'key1') ORDER BY n DESC" against the envtest cluster
  Then the exit code is 0
  And the output produces JQ ".[0].val == \"value1\""
  And the output produces JQ ".[0].n >= 1"

Scenario: use map_get with key with dot in config maps
  When I run kubectl-sql "SELECT map_get(data, 'config.json') AS val FROM cm LIMIT 1" against the envtest cluster
  Then the exit code is 0
  And the output produces JQ ".[0].val == \"{\\\"foo\\\": \\\"bar\\\"}\""


  Scenario: SELECT a map key with bracket notation returns nginx
    When I run kubectl-sql "SELECT DISTINCT metadata.labels['app'] AS app FROM pods" against the envtest cluster
    Then the exit code is 0
    And the output produces JQ "[.[].app] | any(. == \"nginx\")"

  # Scenario: WHERE on a map key with bracket notation filters correctly
  #   When I run kubectl-sql "SELECT name FROM pods WHERE metadata.labels['app'] = 'nginx'" against the envtest cluster
  #   Then the exit code is 0
  #   And the output produces JQ "length > 0"

  # Scenario: SELECT metadata->labels returns the labels map as a JSON object
  #   When I run kubectl-sql "SELECT DISTINCT metadata->labels AS labels FROM pods LIMIT 1" against the envtest cluster
  #   Then the exit code is 0
  #   And the output produces JQ ".[0].labels | has(\"app\")"

  # Scenario: SELECT metadata.labels.* returns the labels map as a JSON object
  #   When I run kubectl-sql "SELECT DISTINCT metadata.labels.* AS labels FROM pods LIMIT 1" against the envtest cluster
  #   Then the exit code is 0
  #   And the output produces JQ ".[0].labels | has(\"app\")"