#!/usr/bin/env sh
set -eu

prometheus_url="${PROMETHEUS_URL:-http://localhost:9090}"
alert_name="PortfolioAPIHigh502Rate"
attempt=1

while [ "$attempt" -le 18 ]; do
  if response=$(curl --fail --silent --show-error \
    "$prometheus_url/api/v1/alerts" 2>/dev/null); then
    if ! printf '%s' "$response" | grep -q "\"alertname\":\"$alert_name\""; then
      rm -f .local/incidents/active.json
      echo "Portfolio API recovered and the incident alert cleared."
      exit 0
    fi
  fi
  sleep 5
  attempt=$((attempt + 1))
done

echo "Timed out waiting for $alert_name to clear." >&2
exit 1
