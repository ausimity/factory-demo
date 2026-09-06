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

## 4. Trigger a realistic break (5 minutes)

```bash
make demo-break-upstream
grep 'portfolio aggregation failed' .local/logs/portfolio-api.jsonl | tail -1 | jq
```

Show that the customer gets a controlled `502`, not bad financial data. Show the
trace and the incompatible v2 fixture. Do not explain the implementation fix.

## 5. Start the Mission (15 to 25 minutes)

Enter `/missions`, then provide the contents of
`docs/missions/01-core-v2.md`. During planning:

- let Droid discover the repository evidence;
- ask it to explain the blast radius and milestones;
- confirm that it preserves the public contract;
- highlight feature and validation workers;
- approve the plan and monitor Mission Control.

When complete, review the changed adapter, contract tests, validation evidence,
and running v2 stack.

## 6. Close with the differentiation (5 minutes)

- Code completion tools can propose the DTO mapping.
- Factory used durable context to scope the change, planned across code, tests,
  docs, and runtime verification, then produced reviewable evidence.
- The same repository can accept the insights product request without rebuilding
  context from scratch.
- Show `docs/missions/02-personalized-insights.md` as the next backlog item.

## Recovery

- Restore the healthy baseline: `make demo-reset`
- Restart everything: `make down && make demo-baseline`
- Check runtime state: `docker compose ps`
- Re-run local validation: `make validate`

Run the complete sequence once before the interview so images are cached.
