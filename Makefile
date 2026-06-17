BINARY := ./bin/kubectl-sql
MODULE := github.com/ebuildy/kubectl-sql

GOLANGCI_LINT_VERSION := v2.12.2

.PHONY: build install lint test test-integration coverage e2e e2e-run-fake dev-deps generate web-assets

WEB_DIR := internal/adapter/web

dev-deps:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	setup-envtest use
	go mod download

generate: web-assets
	go run ./tools/genk8sschema \
		-in internal/adapter/datasources/k8s/testdata/swagger.json \
		-out internal/adapter/datasources/k8s/schema_swagger_k8s_standard_resources.go

# web-assets minifies the editable front-end sources (assets/) into the embedded
# build output (dist/). The minifier runs via `go tool`, so it is a build-time
# dependency only and is not linked into the kubectl-sql binary. Edit files
# under $(WEB_DIR)/assets/, run this target, and commit both.
web-assets:
	go tool minify -o $(WEB_DIR)/dist/index.html $(WEB_DIR)/assets/index.html
	go tool minify -o $(WEB_DIR)/dist/app.css    $(WEB_DIR)/assets/app.css
	go tool minify -o $(WEB_DIR)/dist/app.js     $(WEB_DIR)/assets/app.js

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build: web-assets
	go build -ldflags "-X $(MODULE)/cmd.version=$(VERSION)" -o $(BINARY) .

install: build
	cp $(BINARY) ~/bin/kubectl-sql

lint: web-assets
	golangci-lint run ./...

cyclo:
	gocyclo -top 20 -ignore "_test|Godeps|vendor/|external/" .

test: web-assets
	go test ./... -race -count=1

test-integration:
	KUBEBUILDER_ASSETS="$$(setup-envtest use -p path --installed-only)" go test -tags integration ./test/integration/... -v -count=1

e2e-run-fake: build
	KUBEBUILDER_ASSETS="$$(setup-envtest use -p path --installed-only)" go test -tags integration ./test/integration/... -v -count=1

e2e: build
	go test -tags e2e ./test/e2e/... -v

coverage:
	go test ./... -race -count=1 -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html
