# Factory interview demo guide

Target length: 45 to 60 minutes. Keep the live work focused on one Mission. Use
the insights request as a follow-up or a second run if time permits.

## 1. Frame the problem (5 minutes)

- Financial channels need a stable customer view over systems that change at
  different speeds.
- The difficult part is not generating a handler. It is changing software safely
  with enough repository context, evidence, and governance.
- Factory's value in this demo is repository readiness, Mission planning and
  execution, and verifiable review gates.

## 2. Show the baseline system (10 minutes)

```bash
make demo-baseline
curl --silent http://localhost:8080/api/v1/customers/cust-1001/portfolio | jq
```

Open Grafana and Jaeger. Point out that the stack is local and deterministic,
logs are readable from `.local/logs`, and no credentials are involved.

## 3. Show agent readiness (10 minutes)

- Open `AGENTS.md` and highlight exact commands, architecture boundaries, safety
  rules, and completion evidence.
- Open the test strategy, runbook, OpenAPI contract, and issue templates.
- Run `/readiness-report` after the published repository has an `origin`.
- Explain that the readiness model turns vague "AI-friendly" claims into
  measurable engineering foundations.

## 4. Let telemetry detect an incident (5 minutes)

```bash
make demo-trigger-incident
cat .local/incidents/active.json | jq
```

Show the firing Prometheus alert and controlled customer-facing `502`. Do not
show the fault-injection mechanism, detailed application error, or incompatible
fixture. The audience and Mission should begin with symptoms and impact.

## 5. Start the Mission (15 to 25 minutes)

Enter `/missions`, then provide the contents of
`docs/missions/01-portfolio-incident.md`. During planning:

- let Droid discover the repository evidence;
- require it to separate diagnosis, remediation, and validation milestones;
- ask it to explain its evidence and blast radius;
- confirm that it preserves the public contract;
- highlight feature and validation workers;
- approve the plan and monitor Mission Control.

When complete, review the diagnosis, corrective change, regression tests,
validation evidence, and recovered running stack.

## 6. Close with the differentiation (5 minutes)

- Code completion tools can propose the DTO mapping.
- Factory used durable context to scope the change, planned across code, tests,
  docs, and runtime verification, then produced reviewable evidence.
- The same repository can accept the insights product request without rebuilding
  context from scratch.
- Show `docs/missions/02-personalized-insights.md` as the next backlog item.

## Recovery

- Restore the healthy baseline: `make demo-reset`. This is a Core v1 rollback
  only: it recreates the Core mock on the v1 contract, waits for readiness, and
  runs the end-to-end checks. It never clears `.local/incidents/active.json`,
  never modifies incident archives, and never invokes the recovery helper.
- Resolve an active incident instead with the evidence-backed procedure in
  [`docs/runbooks/portfolio-availability.md`](runbooks/portfolio-availability.md).
  Only `scripts/wait-for-recovery.sh` clears the active-incident marker, and
  only after durable, hash-verified recovery evidence exists and the alert is
  verified healthy and inactive; it fails closed otherwise.
- Restart everything: `make down && make demo-baseline`
- Check runtime state: `docker compose ps`
- Re-run local validation: `make validate`

Run the complete sequence once before the interview so images are cached.
