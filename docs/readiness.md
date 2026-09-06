# Agent readiness evidence

This map makes repository signals easy for humans and agents to discover. The
Factory report remains the authoritative score.

| Pillar | Evidence |
| --- | --- |
| Style and validation | `Makefile`, `.golangci.yml`, `.pre-commit-config.yaml` |
| Build system | `go.mod`, `go.sum`, `Makefile`, multi-stage `Dockerfile` |
| Testing | Unit, adapter, handler, contract, race, and `scripts/e2e.sh` |
| Documentation | `README.md`, `AGENTS.md`, architecture, ADRs, runbook |
| Development environment | `compose.yaml`, `.devcontainer/`, `.env.example` |
| Debugging and observability | JSON file logs, health, Prometheus, Grafana, Jaeger |
| Security | `SECURITY.md`, Gitleaks rules, CODEOWNERS, hardened containers |
| Task discovery | Structured bug and feature templates, PR template |
| Product and experimentation | Mission feature includes a measurable insight event |

## Repository-level prerequisites

`/readiness-report` needs an `origin` remote. After creating the remote:

```bash
git remote add origin <repository-url>
```

Organization settings cannot be proven by repository files. Configure branch
protection, required owner review, and any organization-wide secret scanning in
the source control platform before the final readiness report.

The repository intentionally has no hosted CI workflow because this demo's
delivery constraint is local and credential-free. `make validate` is the single
local gate. Hosted CI can be added later without changing the application.
