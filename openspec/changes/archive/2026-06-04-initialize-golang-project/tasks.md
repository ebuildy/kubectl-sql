## 1. Go Module

- [x] 1.1 Run `go mod init github.com/ebuildy/kubectl-sql` to create `go.mod`
- [x] 1.2 Run `go get github.com/spf13/cobra` to add cobra dependency
- [x] 1.3 Run `go get github.com/cube2222/octosql` to add octosql dependency
- [x] 1.4 Run `go get k8s.io/client-go@latest` to add client-go dependency
- [x] 1.5 Run `go get k8s.io/apimachinery@latest` to add apimachinery dependency
- [x] 1.6 Run `go get github.com/cucumber/godog` to add godog dependency
- [x] 1.7 Run `go mod tidy` to produce a clean `go.sum`

## 2. Package Stubs

- [x] 2.1 Create `internal/parser/parser.go` with `package parser` declaration
- [x] 2.2 Create `internal/planner/planner.go` with `package planner` declaration
- [x] 2.3 Create `internal/executor/executor.go` with `package executor` declaration
- [x] 2.4 Create `internal/k8s/client.go` with `package k8s` declaration
- [x] 2.5 Create `internal/output/renderer.go` with `package output` declaration
- [x] 2.6 Create `internal/debug/enricher.go` with `package debug` declaration
- [x] 2.7 Create `pkg/sqlschema/schema.go` with `package sqlschema` declaration

## 3. CLI Entrypoint

- [x] 3.1 Create `cmd/root.go` with cobra root command named `kubectl-sql` and short description
- [x] 3.2 Add all persistent flags to root command: `--output/-o` (default `table`), `--context`, `--namespace/-n`, `--kubeconfig`
- [x] 3.3 Add remaining flags: `--page-size` (default `500`), `--timeout` (default `30s`), `--no-color`, `--explain`, `--dry-run`
- [x] 3.4 Create `main.go` that calls `cmd.Execute()` and exits on error

## 4. Build Infrastructure

- [x] 4.1 Create `Makefile` with `build` target that produces `./bin/kubectl-sql`
- [x] 4.2 Add `install` target that copies binary to `~/bin/kubectl-sql`
- [x] 4.3 Add `lint` target: `golangci-lint run ./...`
- [x] 4.4 Add `test` target: `go test ./... -race -count=1`
- [x] 4.5 Add `test-integration` target: `go test ./test/integration/... -race -count=1`
- [x] 4.6 Add `coverage` target that generates and opens HTML coverage report
- [x] 4.7 Create `bin/` directory with a `.gitkeep` (add `bin/kubectl-sql` to `.gitignore`)

## 5. Linter

- [x] 5.1 Create `.golangci.yml` enabling: `errcheck`, `govet`, `staticcheck`, `unused`, `gofmt`, `goimports`, `misspell`, `revive`
- [x] 5.2 Set `run.timeout = 5m` and `issues.max-issues-per-linter = 0` in `.golangci.yml`
- [x] 5.3 Verify `make lint` passes on the scaffold (exit 0)

## 6. Unit Test Skeleton

- [x] 6.1 Add `github.com/stretchr/testify` to `go.mod` (`go get github.com/stretchr/testify`)
- [x] 6.2 Create `internal/parser/parser_test.go` with a single `TestPlaceholder` that calls `t.Log("placeholder")` — ensures `make test` runs something
- [x] 6.3 Verify `make test` exits 0 and reports at least one test run

## 7. Devcontainer

- [x] 7.1 Create `.devcontainer/devcontainer.json` using image `mcr.microsoft.com/devcontainers/go:1.26`
- [x] 7.2 Add features: `ghcr.io/devcontainers/features/kubectl-helm-minikube:1` (provides kubectl + helm + minikube) and `ghcr.io/devcontainers/features/go:1` (ensures correct Go toolchain)
- [x] 7.3 Add `postCreateCommand`: `go mod download && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
- [x] 7.4 Add VS Code extensions: `golang.go`, `ms-kubernetes-tools.vscode-kubernetes-tools`

## 8. End-to-End Tests

- [x] 8.1 Create `test/e2e/` directory with a `features/` subdirectory
- [x] 8.2 Create `test/e2e/features/help.feature` with a single scenario: invoking `kubectl-sql --help` exits 0 and prints usage
- [x] 8.3 Create `test/e2e/main_test.go` with a `TestMain` that initialises godog with the `features/` folder and runs the suite
- [x] 8.4 Create `test/e2e/steps_test.go` with step definitions for: running the binary, capturing stdout/stderr, asserting exit code, asserting output contains a string
- [x] 8.5 Add `e2e` Makefile target: `go test ./test/e2e/... -v` (requires `./bin/kubectl-sql` to exist — depend on `build`)
- [x] 8.6 Verify `make e2e` passes (help scenario goes green)

## 9. Supporting Files

- [x] 9.1 Create `docs/grammar.ebnf` with a skeleton comment block noting it is to be filled in
- [x] 9.2 Create `test/unit/.gitkeep`, `test/integration/.gitkeep`, `test/fixtures/.gitkeep`
- [x] 9.3 Create `.gitignore` covering `bin/kubectl-sql`, `coverage.out`, `coverage.html`

## 10. Verification

- [x] 10.1 Run `go build ./...` — must exit 0
- [x] 10.2 Run `make build` — must produce `./bin/kubectl-sql`
- [x] 10.3 Run `./bin/kubectl-sql --help` — must print usage with all 9 flags and exit 0
- [x] 10.4 Run `make lint` — must exit 0
- [x] 10.5 Run `make test` — must exit 0 and report at least one test
- [x] 10.6 Run `make e2e` — help scenario must pass
