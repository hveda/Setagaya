# 奔流 (Honryu) — developer Makefile
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
	$(GO) test -p 1 -count=1 -timeout 30m -tags=integration ./internal/adapters/... ./cmd/...

.PHONY: engine
engine: ## drive real load-test engines through the compiler (needs bzt on PATH)
	$(GO) test -count=1 -timeout 25m -tags=engine ./test/engine/...

.PHONY: e2e
e2e: ## full-stack end-to-end tests (needs Docker)
	$(GO) test -p 1 -count=1 -timeout 30m -tags=e2e ./test/e2e/...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: build
build:
	$(GO) build ./...
