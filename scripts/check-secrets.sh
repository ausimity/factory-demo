#!/usr/bin/env sh
set -eu

if command -v gitleaks >/dev/null 2>&1; then
  exec gitleaks git --staged --config .gitleaks.toml
fi

if git diff --cached --no-ext-diff -U0 | grep -E '^\+.*(BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|AKIA[0-9A-Z]{16}|password[[:space:]]*=[[:space:]]*[^[:space:]]+)' >/dev/null; then
  echo "Potential secret found in staged changes." >&2
  exit 1
fi

echo "No likely secrets found (install gitleaks for the full ruleset)."
