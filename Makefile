.DEFAULT_GOAL := help

.PHONY: help deps build run test test-unit test-integration test-e2e lint fmt check-fmt \
	quality security check-agents check-deps validate up down logs demo-baseline \
	demo-trigger-incident demo-reset generate-openapi

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

test: ## Run all Go tests with race detection, timing reports, and coverage enforcement.
	mkdir -p artifacts
	go tool gotestsum --format testname --jsonfile artifacts/test-results.json \
		--junitfile artifacts/junit.xml -- \
		-race -count=1 -coverprofile=artifacts/coverage.out ./...
	./scripts/check-coverage.sh artifacts/coverage.out 50

test-unit: ## Run fast unit tests.
	go test -race -count=1 ./cmd/core-mock ./cmd/market-mock ./internal/app/... \
		./internal/domain/... ./internal/mockhttp/... ./internal/portfolio/...

test-integration: ## Run adapter and HTTP integration tests.
	go test -race -count=1 ./internal/upstream/... ./internal/httpapi/... ./internal/contracts/...

test-e2e: ## Exercise the running baseline stack.
	./scripts/e2e.sh

fmt: ## Format Go source.
	gofmt -w $$(find cmd internal -name '*.go' -type f)

check-fmt: ## Fail when Go source is not formatted.
	@test -z "$$(gofmt -l $$(find cmd internal -name '*.go' -type f))" || \
		(echo "Run 'make fmt' to format these files:"; gofmt -l $$(find cmd internal -name '*.go' -type f); exit 1)

lint: check-fmt ## Run static analysis.
	go vet ./...

quality: ## Enforce complexity and duplicate-code limits.
	go tool gocyclo -over 15 cmd internal
	@set -e; duplicates="$$(go tool dupl -plumbing -t 100 cmd internal)"; \
		if [ -n "$$duplicates" ]; then \
			echo "Duplicate blocks exceed the 100-token limit:"; \
			echo "$$duplicates"; \
			exit 1; \
		fi

security: ## Produce dependency-vulnerability and static-security reports.
	mkdir -p artifacts
	@go tool govulncheck ./... > artifacts/govulncheck.txt || \
		{ status=$$?; cat artifacts/govulncheck.txt; exit $$status; }
	@cat artifacts/govulncheck.txt
	go tool gosec -fmt=text -out=artifacts/gosec.txt ./...

check-agents: ## Verify documented AGENTS.md commands and repository paths.
	./scripts/check-agents.sh

check-deps: ## Fail if go mod tidy would change dependency manifests.
	go mod tidy -diff

validate: lint quality check-agents check-deps test build ## Run the required local quality gate.
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

demo-trigger-incident: ## Inject an undisclosed fault and wait for telemetry detection.
	@./scripts/wait-for-api.sh http://localhost:9090/-/ready
	@sleep 6
	@CORE_CONTRACT_VERSION=v2 docker compose up --build -d --force-recreate core-api
	@./scripts/wait-for-api.sh
	@for request in 1 2 3 4 5; do \
		curl --silent --output /dev/null \
			http://localhost:8080/api/v1/customers/cust-1001/portfolio; \
	done
	@./scripts/wait-for-incident.sh
	@echo "Customer impact is active. Begin with .local/incidents/active.json."

demo-reset: ## Restore the baseline v1 upstream contract.
	CORE_CONTRACT_VERSION=v1 docker compose up --build -d --force-recreate core-api
	./scripts/wait-for-api.sh
	./scripts/e2e.sh
	./scripts/wait-for-recovery.sh

generate-openapi: ## Validate all hand-maintained API contracts.
	go test ./internal/contracts/...
	@echo "The OpenAPI files in api/ are valid; no generated files changed."
