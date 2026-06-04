## Context

The existing e2e suite uses godog + the `e2e` build tag and tests only CLI surface (help, invalid SQL). The dynamic client path has never been exercised in an automated test. `envtest` from `sigs.k8s.io/controller-runtime` runs a real kube-apiserver binary in-process, with no external cluster required, and is the de-facto standard for Go k8s integration tests.

## Goals / Non-Goals

**Goals:**
- Start an envtest API server once per test suite in `TestMain`, seed fixtures, run all scenarios, then stop
- Fixture data: 10 namespaces with random names, each containing 3–5 Pods, 1–2 Deployments, 1–2 ConfigMaps
- Pods created with `status.phase = Running` via status subresource PATCH using the dynamic client
- Scenarios exercise: SELECT with WHERE filter, LIMIT, cross-namespace listing, namespace-scoped query
- `make e2e-run-fake` runs `go test -tags integration ./test/integration/... -v`
- `//go:build integration` tag gates all files so `make test` and `make e2e` are unaffected
- Works in GitHub Actions CI with no cluster (envtest downloads kube-apiserver via `setup-envtest`)

**Non-Goals:**
- Node simulation or pod scheduling (not needed for LIST queries)
- Testing write operations (kubectl-sql is read-only)
- Multiple concurrent API servers

## Decisions

### envtest over kwokctl

`kwokctl` requires an external binary and wraps multiple processes (etcd + kube-apiserver + kwok controller). `envtest` is a single Go import that downloads and runs just kube-apiserver + etcd via `setup-envtest`. It's the standard for controller-runtime projects and works in CI without any pre-installed tooling.

**Alternative considered**: kwokctl subprocess. Rejected — requires installing an external binary, doesn't work in CI by default, and offers no advantage for a read-only LIST-only tool.

### envtest API server started in TestMain, kubeconfig written to temp file

`envtest.Environment{}.Start()` returns a `*rest.Config`. We write this to a temp kubeconfig file and store the path in a package-level `envKubeconfig` var. The godog step definitions build the CLI command with `--kubeconfig <envKubeconfig>`.

### Fixture generation with `math/rand` + embedded word list

Random names as `<adjective>-<noun>-<4-hex-chars>` using a small embedded word list. No external dependency, produces readable test output. Seed is fixed (`rand.New(rand.NewSource(42))`) for reproducibility within a run but different across runs.

### Pod status via status subresource PATCH

envtest's API server does not run a scheduler or kubelet — pods stay `Pending` unless status is patched manually. After creating each pod, we PATCH its `/status` subresource to set `phase: Running`. This is standard envtest practice.

### Package: `test/integration/` with `//go:build integration`

Aligns with the `test/integration/` directory already scaffolded in AGENTS.md. The build tag `integration` is conventional for envtest suites. Feature files live in `test/e2e/features/` alongside existing Gherkin files.

### make e2e-run-fake

```makefile
e2e-run-fake: build
    KUBEBUILDER_ASSETS=$(shell setup-envtest use --bin-path) \
    go test -tags integration ./test/integration/... -v
```

`setup-envtest use --bin-path` downloads and caches the matching kube-apiserver binary and prints its path. envtest reads `KUBEBUILDER_ASSETS` to find the binary.

## Risks / Trade-offs

- **setup-envtest not installed** → `make e2e-run-fake` fails. Mitigation: devcontainer installs it; CI step can run `go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest`.
- **Slow startup (~10-15s)** → acceptable since the server starts once per suite, not per scenario.
- **Fixture non-determinism** → random names mean assertions count rows, not match exact names. Mitigation: seed data shape is deterministic (10 namespaces × known range), so row-count assertions are reliable.
- **Status PATCH** → pods may show `Pending` if PATCH is missed. Mitigation: fixture code always PATCHes status; tests assert on known phases.

## Open Questions

- Which kube-apiserver version to pin in `setup-envtest`? Use `latest` for now; pin after first failure.
