# Security policy

## Scope

This project is a local demonstration and uses only synthetic data. Report a
security issue privately to the repository owner rather than in a public issue.

## Controls

- Containers run as a non-root user with a read-only filesystem and without
  privilege escalation.
- Public inputs are allow-listed and upstream calls use bounded timeouts.
- Upstream response sizes are limited and payloads are schema-checked.
- Error responses redact internal and upstream details.
- Logs exclude payloads, customer identifiers, holdings, credentials, and
  authorization headers.
- Money uses integer minor units rather than floating-point arithmetic.
- `.env` files and runtime logs are excluded from version control.
- Pre-commit secret scanning uses Gitleaks when available and a conservative
  local fallback otherwise.
- Hosted and local security checks run Gosec against source and govulncheck
  against reachable dependency code. `make security` writes reviewable reports
  under `artifacts/`; hosted CI retains them for 30 days.

## Required review

Changes to public contracts, financial calculations, input validation, logging,
container security, or dependency boundaries require the matching CODEOWNERS
review. Run `make validate` before review.

## Non-goals

Authentication and authorization are outside this local demo's scope. Do not
deploy it to a public network or add real financial data.
