# Telemetry-triggered incident response

## What Factory supports

Factory documents event-triggered Incident Response through Slack:

```text
Prometheus -> Alertmanager -> Slack alert channel
                               |
                               v
                     Factory Incident Response
                               |
                               v
                    linked Droid investigation
```

An alert bot posts a top-level Slack message, and a configured Incident Response
automation starts a Droid session on a selected computer with the alert context.
This feature is currently documented as Private Preview.

Factory does not currently document Prometheus or Alertmanager as a direct
Mission trigger. Incident Response starts an investigation session, while a
Mission is a bounded orchestrated project with an approved plan.

## Recommended live-demo flow

Use a real telemetry signal but launch the Mission manually:

1. `make demo-baseline`
2. `make demo-trigger-incident`
3. Show the firing Prometheus alert and `.local/incidents/active.json`.
4. Enter `/missions`.
5. Provide `docs/missions/01-portfolio-incident.md`.

This preserves the important causal chain, telemetry detects the customer
impact, without making the interview depend on Slack, cloud credentials, private
preview access, or webhook timing.

## Production automation options

### Incident Response automation

Route the Alertmanager notification to a dedicated Slack alert channel. Configure
Factory's Incident Response template to watch bot messages in that channel and
run on a Droid Computer with the repository and observability access. The
resulting Droid session can investigate, perform RCA, and prepare a fix.

### Headless Mission bridge

If the response must use Mission orchestration, deploy a controlled webhook
receiver that validates Alertmanager signatures, deduplicates alerts, checks
cooldown and concurrency policy, and invokes:

```bash
droid exec \
  --mission \
  --auto high \
  --cwd /workspace/factory-demo \
  -f docs/missions/01-portfolio-incident.md
```

Run the bridge on an isolated Droid Computer under a service account. Require:

- alert fingerprint deduplication and one active Mission per service;
- a bounded execution timeout and retry policy;
- repository locking or isolated worktrees;
- explicit commit, pull request, and deployment policies;
- audit logs linking the alert, Mission, validation, and resulting change;
- escalation instead of code changes when diagnosis confidence is low.

Do not run this bridge from a developer laptop or allow every alert to create a
Mission. Alerting remains the trigger; Mission planning and validation remain
the bounded remediation workflow.

## Demo alert

Prometheus evaluates `deploy/prometheus-alerts.yaml`. Five synthetic failed
requests cause `PortfolioAPIHigh502Rate` to fire. The detector writes a generic,
root-cause-neutral alert to `.local/incidents/active.json`; detailed evidence
remains available in metrics, traces, and structured logs for the Mission to
discover.
