# Spec: Web UI API

## Purpose

Defines the JSON HTTP API the embedded web UI is built on: `POST /api/query` (run SQL, return
columns/rows or a structured error) and `GET /api/complete` (completion candidates). It also defines
the read-only guardrail that rejects mutating statements so the browser surface cannot trigger
destructive operations.

---

## Requirements

### Requirement: Query endpoint runs SQL and returns JSON

The server SHALL expose `POST /api/query` accepting a JSON body `{ "sql": "<query>" }`. It SHALL
run the query through the existing SQL engine in JSON output mode against the configured cluster
and namespace, and return `200 OK` with a JSON body containing the column names and result rows.

#### Scenario: Successful query

- **WHEN** a client POSTs `{ "sql": "SELECT name, namespace FROM pods" }`
- **THEN** the response status is `200`
- **AND** the body contains the ordered column names and an array of row objects

#### Scenario: Empty result

- **WHEN** a query matches no resources
- **THEN** the response status is `200`
- **AND** the rows array is empty

### Requirement: Query endpoint returns structured errors

When a query fails to parse or execute, `POST /api/query` SHALL return a non-2xx status with a
JSON body describing the error. When the engine produces a typo-correction suggestion, the
response SHALL include the suggestion text and the corrected SQL so the UI can offer it.

#### Scenario: Parse or execution error

- **WHEN** a client POSTs an invalid query
- **THEN** the response status is `400`
- **AND** the body contains a human-readable error message

#### Scenario: Typo with a suggestion

- **WHEN** a query fails because of a single mistyped token that has a close valid match
- **THEN** the response includes the suggestion message and the corrected SQL string

#### Scenario: Malformed request body

- **WHEN** a client POSTs a body that is not valid JSON or is missing `sql`
- **THEN** the response status is `400`
- **AND** no query is executed

### Requirement: Mutating statements are rejected

To keep the browser surface read-only, `POST /api/query` SHALL reject DELETE (and any other
mutating) statements with a `403 Forbidden` and an explanatory message, and SHALL NOT execute
them. Destructive operations remain available only through the CLI's confirmation flow.

#### Scenario: DELETE is blocked

- **WHEN** a client POSTs a `DELETE FROM ...` statement
- **THEN** the response status is `403`
- **AND** no resource is deleted

### Requirement: Completion endpoint returns candidates

The server SHALL expose a completion endpoint (`GET /api/complete`) that accepts the current
editor line and cursor position and returns the list of completion candidates produced by the
existing completion source, so the editor can present the same suggestions as the REPL.

#### Scenario: Candidates for a partial token

- **WHEN** a client requests completion for line `SELECT name FROM po` with the cursor at the end
- **THEN** the response status is `200`
- **AND** the body contains an array of candidate completions (e.g. resource names beginning with `po`)

#### Scenario: No candidates

- **WHEN** there are no completions for the given line and position
- **THEN** the response status is `200`
- **AND** the candidates array is empty

### Requirement: API responses are JSON with appropriate content type

All `/api/*` responses SHALL set `Content-Type: application/json` and return well-formed JSON for
both success and error cases, so the client can parse responses uniformly.

#### Scenario: Content type on success and error

- **WHEN** any `/api/*` endpoint responds
- **THEN** the `Content-Type` header is `application/json`
- **AND** the body is valid JSON
