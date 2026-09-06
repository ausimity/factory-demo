# Mission: Adapt to the core accounts v2 contract

The core accounts provider has released v2 and no longer serves its v1 response
shape. Portfolio requests now return `502 UPSTREAM_FAILURE`.

## Goal

Restore portfolio availability against the v2 core payload while keeping the
public portfolio API unchanged.

## Acceptance criteria

- The core adapter correctly maps `internal/upstream/testdata/core-v2.json`.
- Customer `cust-1001` still returns the same accounts, holdings, and total
  market value against `CORE_CONTRACT_VERSION=v2`.
- The public response continues to conform to `api/openapi.yaml`; no core v2
  names leak beyond the adapter.
- Invalid or incomplete v2 payloads fail closed with a redacted `502`.
- Contract tests cover successful v2 mapping and at least one malformed payload.
- Existing domain and service tests continue to pass.
- Relevant adapter and operational documentation reflects v2.
- `make validate` passes.
- With the v2 stack running, `make test-e2e` passes.

## Constraints

- Do not change the public API shape or financial calculations.
- Do not relax strict contract validation.
- Do not add retries for malformed data.
- Preserve v1 compatibility if it does not complicate the adapter materially;
  make that decision explicit in the Mission plan.
- Keep all data local and synthetic.

## Runtime reproduction

```bash
make demo-baseline
make demo-break-upstream
```

Use the runbook at `docs/runbooks/upstream-contract-failure.md` for evidence and
safe verification.
