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
name or change code before reproducing the failing boundary. Disposition every
hypothesis against the alert, metric, log, trace, and upstream-version evidence
before choosing a recovery path. Contract drift is one hypothesis, not the
default explanation for every incident.

## Preserve evidence before any change

Before any remediation or state-changing action:

1. Record the firing alert, the exact
   `portfolio_http_requests_total{status="502"}` series and its 30-second
   increase, and the query time.
2. Export one complete failing trace and representative redacted log lines.
3. Record every service container ID, the Core contract version, and the
   runtime alert rule.
4. Copy the incident marker, evidence files, and log prefixes into an
   immutable, hash-verified archive under `.local/incidents/archive/`.

Never truncate logs, overwrite existing evidence, or delete volumes. A missing
series, rule, alert, or log entry never counts as zero, inactive, or
successful.

## Safe fix boundary

Keep the correction at the narrowest boundary supported by evidence. Upstream
wire types must not leak into the domain, service, or public HTTP response.
Preserve integer money rules, strict validation, and error redaction.

## Recovery paths

Choose from evidence, not from the alert name.

### Targeted active-v2 remediation

When runtime evidence proves a deterministic contract-decode failure at the
Core adapter and Core must remain on its current contract:

1. Keep the compatibility correction inside `internal/upstream` only and prove
   it with `make validate` before touching the running stack.
2. If the runtime Prometheus rule differs from the checked-in rule in
   `deploy/prometheus-alerts.yaml`, reload the existing Prometheus process in
   place and nothing else:

   ```bash
   docker kill --signal HUP <prometheus-container-id>
   ```

   Never recreate Prometheus: this topology has no persistent TSDB volume, so
   recreation would discard alert rule state and history. Verify after the
   reload that the container ID, start time, and a fixed-range historical query
   are unchanged and that the loaded rule now matches the checked-in rule.
3. Rebuild and replace only the portfolio API, with no dependency recreation:

   ```bash
   CORE_CONTRACT_VERSION=v2 docker compose build portfolio-api
   CORE_CONTRACT_VERSION=v2 docker compose up -d --no-deps portfolio-api
   ```

   Never use `docker compose down`, delete volumes, or recreate any other
   service. Core stays on its existing contract throughout.
4. Define recovery epoch T0 only after Prometheus has scraped the replacement
   process, both health endpoints return their exact bodies, and a direct
   seeded-customer request returns the exact expected HTTP 200 response.
   Readiness alone does not prove recovery.

### v1 rollback (baseline restoration)

`make demo-reset` recreates the Core mock on the v1 contract, waits for
readiness, and runs the end-to-end checks. It is a rollback to the known-good
baseline, not incident resolution: it never clears
`.local/incidents/active.json`, never modifies incident archives, and never
invokes the recovery helper. Use it while a code fix is reviewed or to return
the demo to its starting state.

## Verification

1. `make validate`
2. `make security`
3. Keep the incident conditions active. For active-v2 validation, Core must
   demonstrably remain on the v2 contract.
4. `./scripts/wait-for-api.sh`
5. `make test-e2e`
6. Confirm the public response still matches `api/openapi.yaml`.
7. Confirm traces export and no payload data appears in logs.
8. Counters and target: after T0, `up{job="portfolio-api"}` stays 1, exactly
   one 200 series grows, the raw 502 series stays at its T0 value, and the
   exact 30-second 502 increase returns one series equal to zero.
9. Alert: after the last 502 sample, wait out the 30-second lookback plus up to
   two 5-second scrape/evaluation intervals. `PortfolioAPIHigh502Rate` must be
   healthy and inactive with no matching active alert.
10. Health: repeated spaced polls of `/health/live` and `/health/ready` return
    HTTP 200 with the exact bodies `{"status":"ok"}` and `{"status":"ready"}`.

## Incident marker clearance

Only `scripts/wait-for-recovery.sh` may clear the mutable marker
`.local/incidents/active.json`. The helper fails closed: it requires a durable,
hash-verified incident archive linked to the marker, exactly one healthy and
inactive `PortfolioAPIHigh502Rate` rule, no matching active alert, exactly one
up `portfolio-api` scrape target, and an exact zero on the 30-second 502
increase. Any missing or unhealthy signal retains the marker. `make demo-reset`
never clears it.

## Incident records

This runbook stays root-cause-neutral and reusable. Incident-specific causes
and timestamps live in the local, hash-verified evidence archives (`.local/` is
runtime data and is not committed):

- Diagnosis: `.local/incidents/archive/20260907T014402Z-diagnosis/diagnosis.md`
- Active-v2 recovery:
  `.local/incidents/archive/20260907T042433Z-recovery/RECOVERY.md`
