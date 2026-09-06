# Contributing

## Workflow

1. Start from a focused issue with acceptance criteria.
2. Read `AGENTS.md` and the relevant architecture or runbook documentation.
3. Make the smallest coherent change that satisfies the criteria.
4. Add or update automated tests.
5. Run `make validate`.
6. If runtime behavior changed, run `make demo-baseline` and `make test-e2e`.
7. Complete every applicable section of the pull request template.

## Compatibility

The OpenAPI document is the public contract. Additive changes require matching
contract tests and documentation. Breaking public changes require explicit API
governance approval and a versioning plan.

Upstream payloads are not domain models. Translate them in `internal/upstream`
and preserve the interfaces consumed by `internal/portfolio`.

## Commit and review expectations

- Keep commits scoped and use imperative summaries.
- Never commit real customer data, credentials, or runtime logs.
- Request the owners named in `.github/CODEOWNERS`.
- Include validation evidence and operational impact in the pull request.
- Add a changelog entry for customer-visible behavior.
