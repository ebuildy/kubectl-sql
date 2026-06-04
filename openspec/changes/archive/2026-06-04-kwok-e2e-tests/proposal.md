## Why

The current e2e suite only tests the CLI surface (help output, invalid SQL exit codes) — it has no scenarios that execute real SQL against real Kubernetes objects. Adding an envtest-backed test environment lets us verify the full query path (parse → k8s LIST → table output) without a live cluster, with no external binary dependency.

## What Changes

- Add `make e2e-run-fake` Makefile target that runs the godog suite tagged `integration` against an in-process envtest API server
- Add `test/integration/` package: `TestMain` that starts `envtest.Environment`, seeds fixture data, and stops the server after the suite
- Seed 10 namespaces with random names, each containing a random mix of Pods, Deployments, and ConfigMaps
- Add godog feature file `test/e2e/features/integration.feature` with scenarios that query the seeded data
- Add `//go:build integration` tag to all envtest test files so `make test` and `make e2e` never run them
- Add `setup-envtest` to devcontainer `postCreateCommand` to download the kube-apiserver binary

## Capabilities

### New Capabilities

- `envtest-e2e`: godog scenarios that run SQL queries against an envtest API server seeded with fixture namespaces, pods, deployments, and configmaps

### Modified Capabilities

- `project-scaffold`: new `e2e-run-fake` Makefile target and `setup-envtest` added to devcontainer setup

## Impact

- `Makefile` — new `e2e-run-fake` target
- `.devcontainer/devcontainer.json` — `setup-envtest` added to `postCreateCommand`
- `test/integration/` — new package (main_test.go, steps_test.go, fixture.go, suite_test.go)
- `test/e2e/features/integration.feature` — new feature file
- `go.mod` — adds `sigs.k8s.io/controller-runtime` for envtest
