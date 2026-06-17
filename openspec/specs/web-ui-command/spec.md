# Spec: Web UI Command

## Purpose

Defines the `--ui` / `--ui-address` CLI flags and the local web server's lifecycle: startup banner,
automatic browser launch, optional positional-query pre-load, reuse of the CLI's cluster
configuration, bind-address validation, and graceful shutdown.

---

## Requirements

### Requirement: --ui flag starts the local web server

When invoked with `--ui`, the plugin SHALL start a local HTTP server that serves the web UI and
its JSON API instead of executing a query, and SHALL NOT require a positional SQL argument.

#### Scenario: Start the UI server

- **WHEN** the user runs `kubectl sql --ui`
- **THEN** an HTTP server starts listening on the configured address
- **AND** the process prints the listen URL (e.g. `kubectl-sql UI listening on http://127.0.0.1:8080`) to stderr
- **AND** no query is executed at startup

#### Scenario: Positional query is not required with --ui

- **WHEN** the user runs `kubectl sql --ui` with no positional argument
- **THEN** the command does not error about a missing query
- **AND** the server starts normally

### Requirement: Browser opens automatically on startup

After the server is listening, the command SHALL open the UI in the user's default browser
(best-effort). A failure to launch a browser SHALL NOT be fatal: the command keeps serving and the
listen URL remains available for the user to open manually.

#### Scenario: Default browser is opened

- **WHEN** the user runs `kubectl sql --ui`
- **THEN** the server's URL is opened in the default browser

#### Scenario: Browser launch failure is non-fatal

- **WHEN** the browser cannot be launched (e.g. a headless environment)
- **THEN** the server keeps running and the listen URL is reported for manual navigation

### Requirement: Positional query pre-loads the UI

When a positional SQL argument is given alongside `--ui`, the command SHALL pass it to the page via
the URL's `sql` query string so the editor opens pre-filled with that query (and runs it). The
query is NOT executed by the command itself before the server starts.

#### Scenario: Query argument is forwarded to the page

- **WHEN** the user runs `kubectl sql --ui "SELECT name FROM pods"`
- **THEN** the browser is opened to the UI URL carrying the query in its `sql` parameter
- **AND** the editor opens pre-filled with that query

### Requirement: --ui-address controls the bind address

The plugin SHALL accept a `--ui-address` flag (default `127.0.0.1:8080`) that sets the
`host:port` the UI server binds to. The default SHALL bind to loopback only so the UI is not
exposed on the network unless the user explicitly opts in.

#### Scenario: Custom bind address

- **WHEN** the user runs `kubectl sql --ui --ui-address 127.0.0.1:9999`
- **THEN** the server listens on `127.0.0.1:9999`
- **AND** the printed URL reflects that address

#### Scenario: Address already in use

- **WHEN** the configured address is already bound by another process
- **THEN** the command exits with a non-zero status and an error explaining the bind failure

#### Scenario: Malformed address is rejected

- **WHEN** the user passes a `--ui-address` that is not a valid `host:port` (e.g. missing port, non-numeric or out-of-range port)
- **THEN** the command exits with a non-zero status and a clear validation error naming the bad value
- **AND** no server is started and the cluster is not contacted

### Requirement: Web server reuses existing cluster configuration

The UI server SHALL build its data source and SQL engine from the same configuration as the CLI,
honouring `--kubeconfig`, `--context`, and `--namespace`, so queries run against the same cluster
the user would query from the terminal.

#### Scenario: Namespace flag is respected

- **WHEN** the user runs `kubectl sql --ui -n kube-system`
- **THEN** queries submitted through the UI are scoped to the `kube-system` namespace

#### Scenario: Cluster unreachable at startup

- **WHEN** the cluster cannot be reached while wiring the data source
- **THEN** the command exits with a non-zero status and a clear connection error
- **AND** no server is left listening

### Requirement: Graceful shutdown

The UI server SHALL shut down cleanly when it receives an interrupt signal (Ctrl-C / SIGINT or
SIGTERM), stopping the listener and returning a zero exit status.

#### Scenario: Interrupt stops the server

- **WHEN** the server is running and the user presses Ctrl-C
- **THEN** the HTTP listener stops accepting connections
- **AND** the process exits with status 0
