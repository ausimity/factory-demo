# Mission: Resolve an active portfolio availability incident

Prometheus is reporting a sustained increase in failed portfolio requests.
Customers cannot load their portfolio dashboard, and the public API is returning
controlled `502 UPSTREAM_FAILURE` responses.

The root cause is not known. Treat the alert, runtime telemetry, repository, and
tests as the available incident evidence. Do not assume the failure described in
a test fixture or prior scenario is the active cause without verifying it.

## Goal

Diagnose the incident, restore portfolio availability, preserve financial
correctness and the public API contract, and leave regression evidence that
prevents the same failure from silently recurring.

## Starting evidence

- Active alert: `.local/incidents/active.json`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000/d/portfolio-api`
- Jaeger: `http://localhost:16686`
- Structured logs: `.local/logs/`
- Operational runbook: `docs/runbooks/portfolio-availability.md`

Begin with the alert and runtime evidence. Establish the failing request path
before proposing an implementation.

## Acceptance criteria

- Document the verified root cause and evidence supporting it.
- Customer `cust-1001` returns HTTP `200` with the expected accounts, holdings,
  and total market value of `34375.00 USD`.
- The public response continues to conform to `api/openapi.yaml`; no core v2
  implementation details leak into the public contract.
- The corrective change remains within the narrowest appropriate architecture
  boundary.
- A regression test reproduces the observed failure and proves the fix.
- Invalid or incomplete upstream data still fails closed with a redacted `502`.
- Existing domain and service tests continue to pass.
- Relevant contract and operational documentation reflects the verified cause.
- `make validate` passes.
- `make security` passes.
- With the incident conditions still active, `make test-e2e` passes.
- Successful synthetic requests no longer increase the `502` metric, and the
  alert returns to inactive after its evaluation window.

## Constraints

- Do not change the public API shape or financial calculations.
- Do not relax strict contract validation.
- Do not add retries until evidence shows the failure is transient.
- Make backward-compatibility decisions explicit in the Mission plan.
- Do not silence the alert or weaken its threshold to make validation pass.
- Keep all data local and synthetic.

## Mission planning expectations

Organize the work into distinct diagnosis, remediation, and validation
milestones. Validation must exercise the running system, not only unit tests.
