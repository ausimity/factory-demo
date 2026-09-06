#!/usr/bin/env sh
set -eu

prometheus_url="${PROMETHEUS_URL:-http://localhost:9090}"
alert_name="PortfolioAPIHigh502Rate"
attempt=1

while [ "$attempt" -le 15 ]; do
  response=$(curl --fail --silent --show-error \
    "$prometheus_url/api/v1/alerts" 2>/dev/null || true)
  if printf '%s' "$response" | grep -q "\"alertname\":\"$alert_name\"" &&
    printf '%s' "$response" | grep -q '"state":"firing"'; then
    mkdir -p .local/incidents
    detected_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    cat >.local/incidents/active.json <<EOF
{
  "alert": "$alert_name",
  "status": "firing",
  "severity": "critical",
  "service": "portfolio-api",
  "customerImpact": "Portfolio dashboard requests are unavailable",
  "signal": "At least three HTTP 502 responses were observed in 30 seconds",
  "source": "prometheus",
  "detectedAt": "$detected_at",
  "runbook": "docs/runbooks/portfolio-availability.md"
}
EOF
    echo "Incident detected. Alert context: .local/incidents/active.json"
    exit 0
  fi
  sleep 4
  attempt=$((attempt + 1))
done

echo "Timed out waiting for $alert_name to fire." >&2
exit 1
