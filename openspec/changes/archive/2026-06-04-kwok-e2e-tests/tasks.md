## 1. Dependencies & Devcontainer

- [x] 1.1 Run `go get sigs.k8s.io/controller-runtime` to add envtest dependency
- [x] 1.2 Run `go mod tidy`
- [x] 1.3 Add `setup-envtest` install to `.devcontainer/devcontainer.json` `postCreateCommand`: append `&& go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest`

## 2. Makefile

- [x] 2.1 Add `e2e-run-fake` and `test-integration` to `.PHONY` list in Makefile
- [x] 2.2 Add `e2e-run-fake` target: calls `setup-envtest use --bin-path` to download the kube-apiserver binary, then starts `kube-apiserver` with a temp kubeconfig and prints the kubeconfig path — server runs in foreground until Ctrl-C
- [x] 2.3 Add `test-integration` target: sets `KUBEBUILDER_ASSETS` from `setup-envtest use --bin-path`, then runs `go test -tags integration ./test/integration/... -v`

## 3. Fixture Generator

- [x] 3.1 Create `test/integration/fixture.go` (build tag `//go:build integration`) with an embedded word list (10 adjectives, 10 nouns) and `randomName() string` returning `<adj>-<noun>-<4-hex-chars>` using `math/rand`
- [x] 3.2 Implement `SeedFixtures(ctx context.Context, dynClient dynamic.Interface) ([]string, error)` that creates 10 namespaces with random names and returns the namespace name list
- [x] 3.3 In each namespace create 3–5 Pods (random count 3–5), 1–2 Deployments, 1–2 ConfigMaps — all with random names
- [x] 3.4 After creating each Pod, PATCH its `/status` subresource to set `status.phase = "Running"` using `dynClient.Resource(podsGVR).Namespace(ns).UpdateStatus()`
- [x] 3.5 Write a unit-level test (no build tag, `test/integration/fixture_names_test.go`) for `randomName()` verifying output matches `^[a-z]+-[a-z]+-[0-9a-f]{4}$`

## 4. envtest TestMain

- [x] 4.1 Create `test/integration/main_test.go` (build tag `//go:build integration`) with package-level vars: `envKubeconfig string`, `envNamespaces []string`
- [x] 4.2 In `TestMain`: create `envtest.Environment{CRDDirectoryPaths: nil}`, call `.Start()`, write the returned `*rest.Config` to a temp kubeconfig file using `clientcmd.WriteToFile`
- [x] 4.3 Build a dynamic client from the rest.Config and call `SeedFixtures`, store result in `envNamespaces`
- [x] 4.4 Call `m.Run()` and defer `env.Stop()` so teardown always runs
- [x] 4.5 If `KUBEBUILDER_ASSETS` is not set, print a helpful error and `os.Exit(1)`

## 5. Step Definitions & Suite

- [x] 5.1 Create `test/integration/steps_test.go` (build tag `//go:build integration`) with `testContext` struct: `stdout`, `stderr`, `exitCode string`, `pickedNamespace string`
- [x] 5.2 Implement step `I run kubectl-sql {string} against the envtest cluster` — runs binary with `--kubeconfig <envKubeconfig>`, captures output
- [x] 5.3 Implement step `the output has at least {int} rows` — counts non-header/non-separator lines in stdout
- [x] 5.4 Implement step `the output has at most {int} rows`
- [x] 5.5 Implement step `the output has between {int} and {int} rows`
- [x] 5.6 Implement step `the exit code is {int}`
- [x] 5.7 Implement step `I pick a random fixture namespace` — stores `envNamespaces[0]` in `pickedNamespace`
- [x] 5.8 Register all steps in `InitializeScenario(sc *godog.ScenarioContext)`
- [x] 5.9 Create `test/integration/suite_test.go` (build tag `//go:build integration`) with `TestFeatures` wiring godog to `../e2e/features/integration.feature`

## 6. Feature File

- [x] 6.1 Create `test/e2e/features/integration.feature` with scenario: cross-namespace pod listing returns at least 30 rows and exits 0
- [x] 6.2 Add scenario: namespace-scoped pod query using a fixture namespace returns between 3 and 5 rows
- [x] 6.3 Add scenario: `LIMIT 5` on pods returns at most 5 rows and exits 0
- [x] 6.4 Add scenario: deployments listing returns at least 10 rows and exits 0
- [x] 6.5 Add scenario: configmaps listing returns at least 10 rows and exits 0

## 7. Verification

- [x] 7.1 Run `go build ./...` — exits 0
- [x] 7.2 Run `make test` — exits 0, integration tests NOT executed
- [x] 7.3 Run `make lint` — exits 0
- [x] 7.4 Run `make e2e-run-fake` (requires `setup-envtest` installed) — all 5 scenarios pass
