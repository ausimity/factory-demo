# ADR-001: Isolate upstream contracts behind ports

- Status: Accepted
- Date: 2026-09-05

## Context

The public portfolio shape should remain stable when systems of record evolve.
Using upstream JSON objects throughout the service would couple every layer to
those external contracts.

## Decision

The portfolio service depends on account and price interfaces expressed in
domain types. Each HTTP adapter privately owns its upstream DTOs and translates
them at the boundary.

## Consequences

An upstream contract change should affect one adapter and its tests. Translation
adds some code, but prevents contract churn from leaking into business logic or
the public API.
