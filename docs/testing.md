# Test strategy

## Test layers

| Layer | Command | Evidence |
| --- | --- | --- |
| Domain and service unit tests | `make test-unit` | Money precision and aggregation rules |
| Adapter and handler integration tests | `make test-integration` | Wire-contract mapping and HTTP behavior |
| Full Go suite with race detector | `make test` | Concurrency safety and regressions |
| Static and build gate | `make validate` | Formatting, vet, tests, all binaries |
| Running-system check | `make test-e2e` | Baseline API, health, totals, and metrics |

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
- Run the focused suite while iterating, then `make validate`.
- Run `make test-e2e` for runtime behavior changes.
