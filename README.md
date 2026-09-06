# Customer Portfolio API

A production-shaped presentation API for a fictional financial institution. The
Go service combines account positions from a core banking API with market prices
and exposes a stable customer portfolio view.

This repository is a Factory demonstration environment. It is intentionally
small, but includes the context, validation, observability, and operational
signals an autonomous engineering agent needs to work safely.

> All customers, accounts, holdings, and prices are synthetic. The project does
> not connect to real financial systems or require cloud credentials.

## Quick start

Prerequisites:

- Go 1.26 or later
- Docker with Compose
- `make` and `curl`

Start and verify the complete baseline:

```bash
make demo-baseline
```

Then request the seeded customer's portfolio:

```bash
curl --silent http://localhost:8080/api/v1/customers/cust-1001/portfolio
```

Useful local URLs:

| Surface | URL |
| --- | --- |
| Portfolio API | <http://localhost:8080> |
| Prometheus | <http://localhost:9090> |
| Grafana dashboard | <http://localhost:3000/d/portfolio-api> |
| Jaeger traces | <http://localhost:16686> |

Stop the stack with `make down`.

## What runs

```text
client
  |
  v
portfolio-api :8080
  |              |
  v              v
core-mock      market-mock
:8081          :8082
  |
  +--> structured logs, Prometheus metrics, OpenTelemetry traces
```

- **portfolio-api** is the public backend-for-frontend. It validates identifiers,
  calls both upstreams, calculates values with integer money arithmetic, and
  maps failures to a stable error contract.
- **core-mock** provides deterministic account balances and positions. It can
  switch between the baseline `v1` contract and an incompatible `v2` contract.
- **market-mock** provides deterministic security prices.
- **observability** includes JSON logs on disk, Prometheus, a provisioned Grafana
  dashboard, and OpenTelemetry traces in Jaeger.

See [the architecture guide](docs/architecture.md) for boundaries and decisions.

## Public API

The public source of truth is [`api/openapi.yaml`](api/openapi.yaml). The
deterministic upstream contracts are documented in
[`api/core-mock.openapi.yaml`](api/core-mock.openapi.yaml) and
[`api/market-mock.openapi.yaml`](api/market-mock.openapi.yaml).

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/customers/{customerId}/portfolio` | Aggregated portfolio |
| `GET` | `/health/live` | Process liveness |
| `GET` | `/health/ready` | Dependency readiness |
| `GET` | `/metrics` | Prometheus metrics |

Only `cust-1001` exists in the synthetic data set.

## Development

```bash
make help              # list supported commands
make test-unit         # fast domain and service tests
make test-integration  # adapter and HTTP tests
make validate          # format check, vet, tests, and builds
make security          # source and dependency security reports
make test-e2e          # verify a running baseline stack
```

The exact agent instructions and completion rules live in
[`AGENTS.md`](AGENTS.md). Contributors should also read
[`CONTRIBUTING.md`](CONTRIBUTING.md).

## Factory demo

The repository supports two Mission-ready change scenarios:

1. **Telemetry-detected availability incident.** `make demo-trigger-incident`
   injects a deterministic but undisclosed fault, generates customer traffic,
   and waits for Prometheus to raise an alert. Use
   [`docs/missions/01-portfolio-incident.md`](docs/missions/01-portfolio-incident.md)
   as the root-cause-neutral Mission request.
2. **Personalized insights.** Use
   [`docs/missions/02-personalized-insights.md`](docs/missions/02-personalized-insights.md)
   as a new product request after the integration is healthy.

The full interview storyline is in [`docs/demo-guide.md`](docs/demo-guide.md).
See [`docs/automation/telemetry-trigger.md`](docs/automation/telemetry-trigger.md)
for production telemetry-to-Droid automation options.

## Agent readiness

This repository includes:

- pinned dependencies and deterministic builds;
- an exact `AGENTS.md` project briefing;
- unit, integration, contract, and end-to-end tests;
- a one-command development environment and a dev container;
- hosted CI, enforced coverage, test timing, dependency hygiene, complexity and
  duplicate-code gates;
- automated dependency updates, static security reports, pre-commit hooks, and
  secret scanning;
- structured logs, metrics, distributed tracing, health checks, and a runbook;
- CODEOWNERS, pull request expectations, and structured issue templates.

Factory's readiness report requires a Git repository with an `origin` remote.
After publishing this repository, run `/readiness-report` from a Droid session.
See [`docs/readiness.md`](docs/readiness.md) for the evidence map and remaining
organization-level controls.
