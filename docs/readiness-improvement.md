# Readiness improvement after `/readiness-fix`

Date: 2026-09-06

## Executive summary

Factory's repository readiness workflow moved this project from **Level 3 at
48.2%** to **Level 4 at 66.2%**.

- **Score improvement:** +18.0 percentage points
- **Level improvement:** Standardized to Optimized
- **Applications evaluated:** 3
- **Criteria improved:** 15
- **Public API changes:** None

The exercise demonstrated more than code generation. Factory measured the
repository, identified specific engineering gaps, implemented repository-local
remediation, validated the result, and measured the improvement with the same
readiness model.

## Before and after

| Measure | Before | After | Change |
| --- | ---: | ---: | ---: |
| Readiness level | Level 3 | Level 4 | +1 level |
| Overall score | 48.2% | 66.2% | +18.0 points |
| Unit tests present | 1/3 apps | 3/3 apps | +2 apps |
| Unit tests runnable | 1/3 apps | 3/3 apps | +2 apps |
| Coverage thresholds | 0/3 apps | 3/3 apps | +3 apps |
| API schema documentation | 1/3 apps | 3/3 apps | +2 apps |
| Automated security review | 0/1 | 1/1 | Added |
| Dependency update automation | 0/1 | 1/1 | Added |

## Improvements made

### Quality and validation

- Added a cyclomatic-complexity limit of 15.
- Added duplicate-code detection at a 100-token threshold.
- Added `go mod tidy -diff` dependency hygiene.
- Added automated validation of commands and paths documented in `AGENTS.md`.

### Testing

- Added focused unit tests for both mock services.
- Added a repository coverage gate with a 50% minimum.
- Added retained JUnit, coverage, and JSON timing reports.
- Added a CI summary of the slowest tests.
- Standardized test naming and parallel isolation across all services.

### Contracts and documentation

- Added validated OpenAPI contracts for the core and market mock services.
- Added a contract test that validates all three OpenAPI documents.
- Extended repository instructions and test documentation for the new gates.

### Automation and security

- Added GitHub Actions validation and security jobs.
- Added weekly Dependabot updates for Go modules, GitHub Actions, and Docker.
- Added Gosec source scanning and Govulncheck dependency scanning.
- Upgraded Go from 1.26.1 to 1.26.8 after the corrected security gate found
  standard-library vulnerabilities.
- Upgraded OpenTelemetry, gRPC, `x/net`, Gosec, and Govulncheck to fixed,
  compatible versions.
- Hardened log-file creation against path traversal.

## Verification evidence

The improved repository passed:

- `make validate`
- 28 Go tests with race detection
- **53.8% test coverage** against a 50% required minimum
- complexity and duplicate-code gates
- dependency-manifest validation
- OpenAPI validation for all three services
- Govulncheck with **no known vulnerabilities**
- Gosec with **zero findings**
- a fresh local end-to-end portfolio request

The initial generated security and duplicate-code commands returned success
without reliably enforcing their intended policy. The review step caught these
false-positive gates. They were corrected before the final validation and
readiness report. This is an important part of the demonstration: autonomous
implementation remained subject to independent, executable verification.

## Exact criteria gains

The second readiness report recorded these changes:

- Cyclomatic complexity: `0/3` to `3/3`
- Duplicate-code detection: `0/3` to `3/3`
- Build performance tracking: `0/1` to `1/1`
- Unused dependency detection: `0/3` to `3/3`
- Unit tests exist: `1/3` to `3/3`
- Unit tests runnable: `1/3` to `3/3`
- Test performance tracking: `0/3` to `3/3`
- Test coverage thresholds: `0/3` to `3/3`
- Test naming conventions: `1/3` to `3/3`
- Test isolation: `1/3` to `3/3`
- API schema documentation: `1/3` to `3/3`
- `AGENTS.md` validation: `0/1` to `1/1`
- Code quality metrics: `0/3` to `3/3`
- Automated security review: `0/1` to `1/1`
- Dependency update automation: `0/1` to `1/1`

## Remaining opportunities

The Level 4 report recommends:

1. Add tracing, metrics, dashboards, and alerts for both mock services.
2. Add automated PR review and protect `main`.
3. Add dynamic security testing, release automation, and generated release
   notes.

Some gaps are intentionally outside this local, credential-free demo, including
production deployment frequency, progressive rollout, and production error
tracking.

## Slide-ready takeaway

> Factory turned repository readiness into a measurable engineering loop:
> assess, remediate, independently verify, and remeasure. In one pass, the
> project advanced from Level 3 at 48.2% to Level 4 at 66.2%, while preserving
> the public API and proving the result through tests and security gates.

Latest report:
<https://app.factory.ai/analytics/readiness/https%253A%252F%252Fgithub.com%252Fausimity%252Ffactory-demo>
