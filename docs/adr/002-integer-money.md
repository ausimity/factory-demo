# ADR-002: Use integer minor units for money

- Status: Accepted
- Date: 2026-09-05

## Context

Binary floating point cannot exactly represent many decimal currency values.
Rounding errors are unacceptable in financial presentation data.

## Decision

Domain money values use signed 64-bit integer minor units and an ISO-style
three-letter currency. Quantities support at most six decimal places and must
produce an exact minor-unit market value.

## Consequences

Calculations are deterministic and currency mismatches fail explicitly. The
current implementation rejects sub-cent market values rather than silently
rounding them.
