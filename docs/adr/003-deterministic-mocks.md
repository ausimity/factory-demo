# ADR-003: Keep upstream simulators deterministic and local

- Status: Accepted
- Date: 2026-09-05

## Context

A live demo cannot depend on internet access, cloud credentials, market hours,
or a third party's availability. Missions also need reproducible failures.

## Decision

Two small Go services simulate core banking and market data with fixed synthetic
fixtures. An environment variable selects the core API contract version.

## Consequences

Every participant can reproduce the same baseline and contract break. The mocks
do not model production scale or every failure mode, and are never deployment
substitutes for real sandbox integrations.
