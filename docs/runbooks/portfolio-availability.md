# Runbook: Portfolio availability degradation

## Signal

Portfolio requests return:

```json
{"error":{"code":"UPSTREAM_FAILURE","message":"portfolio data is temporarily unavailable"}}
```

The HTTP status is `502`. Request telemetry shows an increase in
`portfolio_http_requests_total{status="502"}`.

## Triage

1. Confirm process and dependencies:

   ```bash
   curl --fail http://localhost:8080/health/live
   curl --fail http://localhost:8080/health/ready
   ```

2. Inspect the API's redacted local logs:

   ```bash
   grep 'portfolio aggregation failed' .local/logs/portfolio-api.jsonl | tail
   ```

3. Inspect the failing request trace at <http://localhost:16686> using service
   `portfolio-api`.
4. Check the core mock's active contract:

   ```bash
   docker compose logs --tail=20 core-api
   ```

   Look for `core mock configured` and its `contract_version` field. Do not copy
   upstream payloads into logs or issue comments.

5. Reproduce at the adapter boundary:

   ```bash
   make test-integration
   ```

## Investigation hypotheses

Use evidence to distinguish among dependency unavailability, timeout, malformed
data, missing prices, and contract drift. Do not assume a cause from the alert
name or change code before reproducing the failing boundary.

## Safe fix boundary

Keep the correction at the narrowest boundary supported by evidence. Upstream
wire types must not leak into the domain, service, or public HTTP response.
Preserve integer money rules, strict validation, and error redaction.

## Verification

1. `make validate`
2. Keep the incident conditions active.
3. `./scripts/wait-for-api.sh`
4. `make test-e2e`
5. Confirm the public response still matches `api/openapi.yaml`.
6. Confirm traces export and no payload data appears in logs.

## Rollback

Run `make demo-reset` to restore the healthy baseline while a code fix is
reviewed.
