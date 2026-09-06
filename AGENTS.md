# Repository guide

## What this project is

This repository contains a Go presentation-layer API for a fictional financial
institution. It aggregates account positions from `core-mock` with prices from
`market-mock` and returns a stable customer portfolio view. All data is synthetic.

## Required commands

- Download dependencies: `make deps`
- Start the full local stack: `make up`
- Run focused unit tests: `make test-unit`
- Run integration tests: `make test-integration`
- Run all tests: `make test`
- Run static checks: `make lint`
- Run complexity and duplication checks: `make quality`
- Run source and dependency security scans: `make security`
- Verify dependency manifests: `make check-deps`
- Validate this guide: `make check-agents`
- Build all binaries: `make build`
- Run the complete local gate: `make validate`
- Exercise a running stack: `make test-e2e`

Run `make validate` before declaring any code change complete. Add a regression
test for every bug fix and tests for every new behavior.

## Repository map

- `cmd/portfolio-api/` wires and starts the public API.
- `cmd/core-mock/` and `cmd/market-mock/` are deterministic upstream simulators.
- `internal/domain/` owns financial value types and precision rules.
- `internal/portfolio/` owns aggregation business logic.
- `internal/upstream/` owns external contract adapters.
- `internal/httpapi/` owns public HTTP routing and error mapping.
- `internal/observability/` owns logging, metrics, and tracing setup.
- `api/openapi.yaml` is the hand-maintained public API source of truth.
- `api/*-mock.openapi.yaml` documents the private deterministic mock APIs.
- `deploy/` contains local observability configuration.
- `docs/` contains architecture, runbooks, and demo material.

## Architecture boundaries

- HTTP handlers may call the portfolio service, not upstream clients directly.
- The portfolio service depends on interfaces and domain types, not HTTP payloads.
- Upstream wire formats remain private to their adapter packages.
- Money uses integer minor units internally. Never use floating point for money.
- Public errors use the envelope defined in `api/openapi.yaml`; do not leak
  upstream bodies or internal errors.
- Preserve the public API when an upstream contract changes.

## Coding conventions

- Use the standard library unless an existing dependency already solves the need.
- Keep constructors explicit and return errors rather than panicking.
- Wrap errors with operation context using `%w`.
- Thread `context.Context` through I/O boundaries.
- Log structured operational facts. Never log account payloads, holdings, secrets,
  authorization headers, or other customer data.
- Keep test data synthetic and deterministic.

## Generated files

There are currently no generated files. The OpenAPI files in `api/` are edited
by hand and validated by `internal/contracts/openapi_test.go`.
If generation is introduced, commit both its source and output and document the
exact regeneration command here.

## Security and safety

- Do not add real customer data, credentials, external financial APIs, or cloud
  dependencies.
- Do not weaken timeouts, input validation, error redaction, or non-root
  container execution.
- Do not delete Docker volumes or local files as part of routine validation.
- Get approval before changing the public API contract.

## Completion evidence

A pull request must include the behavior changed, tests added, `make validate`
result, API contract impact, and operational impact. Update relevant docs when
commands, architecture, contracts, or runbooks change.
