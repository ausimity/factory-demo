# Agent readiness evidence

This map makes repository signals easy for humans and agents to discover. The
Factory report remains the authoritative score.

| Pillar | Evidence |
| --- | --- |
| Style and validation | `Makefile`, `.golangci.yml`, complexity and duplication gates |
| Build system | `go.mod`, `go.sum`, `Makefile`, multi-stage `Dockerfile` |
| Testing | Mock unit tests, adapter and handler tests, 50% coverage gate, retained timing reports |
| Documentation | Validated OpenAPI for all services, `AGENTS.md` consistency check, ADRs, runbook |
| Development environment | `compose.yaml`, `.devcontainer/`, `.env.example` |
| Debugging and observability | JSON file logs, health, Prometheus, Grafana, Jaeger |
| Security | Gosec and govulncheck reports, Gitleaks rules, CODEOWNERS, hardened containers |
| Task discovery | Structured bug and feature templates, PR template |
| Product and experimentation | Mission feature includes a measurable insight event |
| Hosted automation | Pull request CI, dependency updates, cached Go builds, retained reports |

## Repository-level prerequisites

`/readiness-report` needs an `origin` remote. After creating the remote:

```bash
git remote add origin <repository-url>
```

Organization settings cannot be proven by repository files. Branch protection,
required owner review, and organization-wide controls remain source-control
settings rather than repository-local configuration. The hosted CI workflow
runs the same `make validate` gate used locally and does not need application
credentials.
