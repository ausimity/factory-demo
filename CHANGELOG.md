# Changelog

All notable customer-visible changes are documented here.

## [Unreleased]

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
