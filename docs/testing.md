# Test strategy

## Test layers

| Layer | Command | Evidence |
| --- | --- | --- |
| Unit tests | `make test-unit` | Mock endpoints, money precision, and aggregation rules |
| Integration tests | `make test-integration` | Wire-contract mapping, HTTP behavior, and valid OpenAPI |
| Full Go suite | `make test` | Race detection, JUnit/JSON timing reports, and at least 50% total coverage |
| Static and build gate | `make validate` | Formatting, vet, complexity, duplication, dependency hygiene, tests, and binaries |
| Security reports | `make security` | Gosec source findings and Go vulnerability results in `artifacts/` |
| Running-system check | `make test-e2e` | Baseline API, health, totals, and metrics |

The hosted CI workflow retains JUnit, Go test JSON, and coverage profiles for
14 days. Its job summary lists the ten slowest tests so regressions in suite
duration are visible during review.

## Contract fixtures

`internal/upstream/testdata/core-v1.json` is the supported baseline contract.
`core-v2.json` is the known incompatible future contract. Before the v2 Mission,
the integration suite proves v2 is rejected rather than silently interpreted as
empty account data.

After adapting to v2, replace that negative expectation with positive mapping
coverage. Preserve v1 compatibility unless the Mission plan explicitly decides
otherwise.

## Completion policy

- Every defect gets a regression test that fails without the fix.
- New public fields require OpenAPI and handler tests.
- New financial rules require table-driven domain or service tests.
- Adapter changes require realistic JSON fixture tests.
- Keep cyclomatic complexity at or below 15 and duplicate blocks below 100
  tokens. Refactor shared behavior rather than suppressing either check.
- Keep total statement coverage at or above 50%.
- Run the focused suite while iterating, then `make validate`.
- Run `make test-e2e` for runtime behavior changes.
