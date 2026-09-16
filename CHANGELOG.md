# Changelog

All notable customer-visible changes are documented here.

## [Unreleased]

### Added

- Deterministic personalized portfolio insights on every successful response: a
  required `insights` array with concentration and cash-buffer observations
  derived from exact integer arithmetic over the finalized portfolio.
- Bounded insight telemetry via the `portfolio_insights_generated_total`
  Prometheus counter, labeled only by the two fixed insight types and
  incremented once per generated insight on successful responses.

Portfolio totals, account and holding values, error behavior, and Core v1/v2
public compatibility are unchanged.

### Fixed

- Restored portfolio availability under the active Core v2 contract by adding
  strict, backward-compatible Core v1/v2 handling inside the private upstream
  adapter. The public API contract, error envelope, and integer-money behavior
  are unchanged.

## [1.0.0] - 2026-09-05

### Added

- Aggregated customer accounts, holdings, and market values.
- Stable OpenAPI contract and redacted error responses.
- Local mock upstreams and operational telemetry.
