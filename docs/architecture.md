# Architecture

## Context

The portfolio API is a backend-for-frontend between a customer dashboard and
institutional systems of record. It gives clients one stable view while upstream
teams can evolve their own contracts.

```text
                          +------------------+
                          | Customer channel |
                          +--------+---------+
                                   |
                            public OpenAPI
                                   |
                          +--------v---------+
                          |  Portfolio API   |
                          | handlers/service |
                          +----+---------+---+
                               |         |
                    account port         price port
                               |         |
                       +-------v--+   +--v---------+
                       | Core API |   | Market API |
                       | adapter  |   | adapter    |
                       +----------+   +------------+
```

## Request flow

1. The HTTP handler validates the customer identifier.
2. The portfolio service asks the account port for balances and positions.
3. The service asks the price port for the unique symbols it discovered.
4. Domain types calculate each holding, account, and portfolio market value.
5. The handler maps domain values into the stable public response.
6. Logs, metrics, and traces record operational facts without recording payloads.

## Boundaries

| Layer | Owns | Must not own |
| --- | --- | --- |
| `internal/httpapi` | Routing, public DTOs, status mapping | Upstream wire formats |
| `internal/portfolio` | Use case and aggregation policy | HTTP or JSON concerns |
| `internal/domain` | Money, quantity, portfolio values | Network clients |
| `internal/upstream` | Private wire formats and translation | Public response DTOs |
| `cmd/*` | Configuration and dependency wiring | Business rules |

These boundaries localize an upstream contract change to its adapter and tests.
That is the core design seam exercised by the first Mission.

### Core contract compatibility

The Core adapter recognizes exactly two private wire contracts and strictly
validates each before mapping: v1 (`customer_id` + `accounts`) and v2
(`client` + `portfolios`). Either maps into the existing `[]domain.Account`
values; mixed, incomplete, or unsupported payloads are rejected before any
Market call. Wire types stay private to `internal/upstream`. This compatibility
leaves the public OpenAPI contract, service and domain boundaries,
integer-minor-unit money, timeout behavior, and error redaction unchanged, and
adds no retries: the observed contract failure was deterministic, not
transient.

## Reliability

- Every outbound request carries a context and a client timeout.
- HTTP servers bound header, read, write, and idle times.
- Responses larger than 1 MiB are not decoded.
- Readiness checks verify both required upstreams.
- Invalid or unavailable upstream data fails closed with a redacted `502`.
- Containers are non-root and read-only.

This demo does not implement retries. Retrying malformed contracts cannot help,
and automatic retries would make the failure less clear during the demonstration.

Incident recovery is serialized: preserve immutable evidence first; reload the
existing Prometheus process in place (SIGHUP) only when its loaded rule differs
from the checked-in rule; replace only the `portfolio-api` container; then
validate counters, alert state, and health. Prometheus is never recreated in
this non-persistent topology, because recreation would discard alert history.

## Observability

- JSON logs go to stdout and `.local/logs/*.jsonl`.
- Prometheus scrapes request totals and request duration.
- Grafana provisions a focused API dashboard.
- OpenTelemetry traces include inbound requests and outbound HTTP calls.
- Health endpoints separate process liveness from dependency readiness.

Synthetic identifiers are not logged. If the system later handles real customer
data, the same prohibition remains mandatory.

## Decisions

- [ADR-001: Ports and adapters](adr/001-ports-and-adapters.md)
- [ADR-002: Integer financial arithmetic](adr/002-integer-money.md)
- [ADR-003: Deterministic local upstreams](adr/003-deterministic-mocks.md)
