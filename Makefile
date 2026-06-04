BINARY := ./bin/kubectl-sql
MODULE := github.com/ebuildy/kubectl-sql

GOLANGCI_LINT_VERSION := v2.10.1

.PHONY: build install lint test test-integration coverage e2e e2e-run-fake dev-deps

dev-deps:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go mod download

build:
	go build -o $(BINARY) .

install: build
	cp $(BINARY) ~/bin/kubectl-sql

lint:
	golangci-lint run ./...

test:
	go test ./... -race -count=1

test-integration:
	$(eval KUBE_ASSETS := $(shell setup-envtest use -p path))
	KUBEBUILDER_ASSETS="$(KUBE_ASSETS)" go test -tags integration ./test/integration/... -v -count=1

e2e-run-fake: build
	$(eval KUBE_ASSETS := $(shell setup-envtest use -p path))
	KUBEBUILDER_ASSETS="$(KUBE_ASSETS)" go test -tags integration ./test/integration/... -v -count=1

e2e: build
	go test -tags e2e ./test/e2e/... -v

coverage:
	go test ./... -race -count=1 -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html
