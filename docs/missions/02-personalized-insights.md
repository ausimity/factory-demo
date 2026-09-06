# Mission: Add personalized portfolio insights

Customers can see balances and holdings but cannot quickly understand what needs
attention. Add deterministic, explainable insights derived from their portfolio.

## User outcome

After viewing a portfolio, a customer sees concise guidance about concentration
and cash allocation without receiving regulated investment advice.

## Acceptance criteria

- The portfolio response gains an `insights` array documented in OpenAPI.
- A holding above 50% of total portfolio market value produces a `CONCENTRATION`
  insight naming the symbol and its percentage.
- Cash at or above 10% of total value produces a `CASH_BUFFER` informational
  insight; cash below 10% produces a warning.
- Rules use integer or rational arithmetic without floating-point money values.
- Insight order is deterministic and every explanation states the rule used.
- The seeded customer produces both concentration and cash-buffer insights.
- A metric counts generated insights by type, without customer labels.
- Unit tests cover threshold boundaries, no-data behavior, and deterministic
  ordering.
- Handler tests and `api/openapi.yaml` cover the additive public fields.
- Architecture and changelog documentation are updated.
- `make validate` and the running `make test-e2e` check pass.

## Constraints

- Use only current portfolio data; do not add an upstream or customer profile.
- Do not call the output financial advice or make buy/sell recommendations.
- Do not log customer identifiers, holdings, or insight text.
- Keep insight policy separate from HTTP mapping and upstream adapters.

## Success signal

Prometheus exposes generated insight counts by bounded insight type. The metric
must not use customer, account, or symbol labels.
