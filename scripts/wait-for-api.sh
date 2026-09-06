#!/usr/bin/env sh
set -eu

url="${1:-http://localhost:8080/health/ready}"
attempt=1
while [ "$attempt" -le 30 ]; do
  if curl --fail --silent --show-error "$url" >/dev/null 2>&1; then
    exit 0
  fi
  sleep 1
  attempt=$((attempt + 1))
done

echo "Timed out waiting for $url" >&2
exit 1
