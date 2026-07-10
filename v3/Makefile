# Setagaya v3 — developer Makefile
# Fast unit tests need no infra. Integration tests use Docker (testcontainers) and
# are gated behind the `integration` build tag.

GO        ?= go
PKG       ?= ./...
COVERPROF ?= coverage.out

.PHONY: all
all: fmt vet test

.PHONY: fmt
fmt:
	$(GO) fmt $(PKG)

.PHONY: vet
vet:
	$(GO) vet $(PKG)

.PHONY: test
test: ## fast unit tests (no infra)
	$(GO) test -race -count=1 $(PKG)

.PHONY: cover
cover: ## unit tests with coverage profile (no infra)
	$(GO) test -race -count=1 -covermode=atomic -coverprofile=$(COVERPROF) $(PKG)
	$(GO) tool cover -func=$(COVERPROF) | tail -1

.PHONY: cover-gate
cover-gate: ## full coverage (unit+integration+e2e) enforcing the threshold (needs Docker)
	./scripts/coverage.sh

.PHONY: integration
integration: ## adapter contract tests against real infra (needs Docker)
	$(GO) test -p 1 -count=1 -tags=integration ./internal/adapters/... ./cmd/...

.PHONY: e2e
e2e: ## full-stack end-to-end tests (needs Docker)
	$(GO) test -p 1 -count=1 -tags=e2e ./test/e2e/...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: build
build:
	$(GO) build ./...
