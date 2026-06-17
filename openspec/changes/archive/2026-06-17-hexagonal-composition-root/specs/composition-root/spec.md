## ADDED Requirements

### Requirement: All adapter construction lives in the composition root
A single package `internal/app` SHALL own the construction of every concrete adapter (the k8s `DataSource`, the SQL engine factory, the mutator, the spellchecker, the completion source, the readline shell, and the web server). The domain command builders exposed by `internal/app` SHALL construct these adapters and inject them, as ports, into the domain command constructors.

#### Scenario: Adapters are constructed only in internal/app
- **WHEN** the source tree is scanned for calls to the adapter constructors (`k8sAdapter.New`, `octosql.NewFactory`/`octosql.New`, `mutatorAdapter.New`, `spellcheckerAdapter.New`, `shellCompletionAdapter.NewShellCompletion`, `shellAdapter`, `webAdapter.NewServer`)
- **THEN** those construction sites appear only within `internal/app` (the domain command packages contain none)

#### Scenario: Domain commands are built by internal/app
- **WHEN** `cmd` needs a query, repl, or ui command
- **THEN** it obtains it from an `internal/app` builder (e.g. `app.NewQueryCommand`), which wires the adapters and injects ports into the domain constructor

### Requirement: The domain depends only on ports
No package under `internal/domain/` SHALL import any package under `internal/adapter/`. Domain command constructors SHALL accept their dependencies as port interfaces (`DataSource`, `EngineFactory`, `Mutator`, `ShellCompletionRunner`) rather than constructing concrete adapters.

#### Scenario: Domain imports no adapter package
- **WHEN** the `internal/domain/...` packages are scanned for imports
- **THEN** no import path under `internal/adapter/` is present

#### Scenario: Query command takes ports
- **WHEN** the query command is constructed
- **THEN** it receives a `DataSource`, an `EngineFactory`, and a `Mutator` as injected port values, and exposes no constructor that builds these adapters itself

### Requirement: cmd is reduced to flag parsing and delegation
The `cmd` package SHALL parse cobra flags into `api.Config` and delegate command construction to `internal/app`. `cmd` SHALL NOT call any adapter constructor directly.

#### Scenario: cmd performs no adapter construction
- **WHEN** the `cmd` package is scanned for imports and calls
- **THEN** it imports `internal/app` (and `internal/port/*`) for wiring but calls no `internal/adapter/*` constructor for data sources, engines, mutators, shells, or servers

### Requirement: Refactor preserves observable behavior
Moving adapter construction into `internal/app` SHALL NOT change any query result, output format, exit code, prompt, or CLI flag. The existing unit and integration suites SHALL pass unchanged.

#### Scenario: Behavior is unchanged after the refactor
- **WHEN** the existing test suite (`make lint test`) is run after the refactor
- **THEN** it passes without modification to test expectations
