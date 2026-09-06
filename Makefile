.DEFAULT_GOAL := help

.PHONY: help deps build run test test-unit test-integration test-e2e lint fmt check-fmt validate \
	up down logs demo-baseline demo-break-upstream demo-reset generate-openapi

help: ## List available commands.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Download pinned Go dependencies.
	go mod download

build: ## Compile every application.
	mkdir -p bin
	go build -trimpath -o bin/portfolio-api ./cmd/portfolio-api
	go build -trimpath -o bin/core-mock ./cmd/core-mock
	go build -trimpath -o bin/market-mock ./cmd/market-mock

run: ## Run the complete stack with Docker Compose.
	$(MAKE) up

test: ## Run all Go tests with race detection.
	go test -race -count=1 ./...

test-unit: ## Run fast unit tests.
	go test -race -count=1 ./internal/domain/... ./internal/portfolio/...

test-integration: ## Run adapter and HTTP integration tests.
	go test -race -count=1 ./internal/upstream/... ./internal/httpapi/...

test-e2e: ## Exercise the running baseline stack.
	./scripts/e2e.sh

fmt: ## Format Go source.
	gofmt -w $$(find cmd internal -name '*.go' -type f)

check-fmt: ## Fail when Go source is not formatted.
	@test -z "$$(gofmt -l $$(find cmd internal -name '*.go' -type f))" || \
		(echo "Run 'make fmt' to format these files:"; gofmt -l $$(find cmd internal -name '*.go' -type f); exit 1)

lint: check-fmt ## Run static analysis.
	go vet ./...

validate: lint test build ## Run the required local quality gate.
	@echo "Validation passed."

up: ## Start the baseline API, upstreams, and observability stack.
	mkdir -p .local/logs
	CORE_CONTRACT_VERSION=v1 docker compose up --build -d
	@echo "API: http://localhost:8080  Grafana: http://localhost:3000  Jaeger: http://localhost:16686"

down: ## Stop the local stack without deleting persistent data.
	docker compose down

logs: ## Follow application logs.
	docker compose logs -f portfolio-api core-api market-api

demo-baseline: up ## Start and verify the healthy phase-one system.
	./scripts/wait-for-api.sh
	./scripts/e2e.sh

demo-break-upstream: ## Switch the core mock to its incompatible v2 contract.
	CORE_CONTRACT_VERSION=v2 docker compose up --build -d --force-recreate core-api
	./scripts/wait-for-api.sh
	@echo "Core API now emits v2. The portfolio request should expose the contract break:"
	@curl --fail-with-body --silent http://localhost:8080/api/v1/customers/cust-1001/portfolio || true
	@echo

demo-reset: ## Restore the baseline v1 upstream contract.
	CORE_CONTRACT_VERSION=v1 docker compose up --build -d --force-recreate core-api
	./scripts/wait-for-api.sh

generate-openapi: ## Validate that the hand-maintained API contract exists.
	@test -s api/openapi.yaml
	@echo "api/openapi.yaml is the source of truth; no generated files changed."
