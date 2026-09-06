#!/usr/bin/env sh
set -eu

commands=$(sed -n 's/.*`make \([a-zA-Z0-9_-]*\)`.*/\1/p' AGENTS.md | sort -u)
if [ -z "$commands" ]; then
  echo "AGENTS.md does not document any make commands." >&2
  exit 1
fi

for target in $commands; do
  if ! make -n "$target" >/dev/null; then
    echo "AGENTS.md references an invalid make target: $target" >&2
    exit 1
  fi
done

for path in \
  cmd/portfolio-api \
  cmd/core-mock \
  cmd/market-mock \
  internal/domain \
  internal/portfolio \
  internal/upstream \
  internal/httpapi \
  internal/observability \
  api/openapi.yaml \
  api/core-mock.openapi.yaml \
  api/market-mock.openapi.yaml \
  deploy \
  docs
do
  if [ ! -e "$path" ]; then
    echo "AGENTS.md references a missing path: $path" >&2
    exit 1
  fi
done

echo "AGENTS.md commands and repository map are valid."
