#!/usr/bin/env sh
# Evidence-preserving, fail-closed clearance of the mutable active-incident
# marker.
#
# The active portfolio incident is only considered resolved when a durable
# archived incident snapshot linked to the current marker exists AND the named
# Prometheus alert rule, scrape target, exact recovery query, and active-alert
# state all independently prove recovery. Only then is the single mutable
# marker `.local/incidents/active.json` removed. Any missing, malformed,
# ambiguous, or unhealthy signal fails closed and leaves the marker and every
# archived byte untouched. Absence never means recovered.
#
# This helper performs one deterministic evaluation. Waiting for the
# scrape/evaluation window to elapse is the caller's responsibility; the helper
# starts no background poller and leaves no process behind.
set -eu

prometheus_url="${PROMETHEUS_URL:-http://localhost:9090}"
incident_marker="${INCIDENT_MARKER:-.local/incidents/active.json}"
archive_dir="${INCIDENT_ARCHIVE_DIR:-.local/incidents/archive}"
alert_name="PortfolioAPIHigh502Rate"
target_job="portfolio-api"
# Semantically equal to the checked-in 30s 502-increase expression, without the
# alerting threshold, so the recovered value can be compared exactly to zero.
recovery_query='sum(increase(portfolio_http_requests_total{status="502"}[30s]))'

fail() {
  echo "Recovery not confirmed: $1 The active incident marker is retained." >&2
  exit 1
}

# Fetch a Prometheus endpoint and require a 2xx response whose body is a
# well-formed JSON object reporting status "success". Echoes the body.
fetch_success() {
  endpoint="$1"
  shift
  if ! body=$(curl --fail --silent --show-error "$@" \
    "$prometheus_url$endpoint" 2>/dev/null); then
    fail "Prometheus was unavailable or returned an error for $endpoint."
  fi
  if ! printf '%s' "$body" | jq -e '.status == "success"' >/dev/null 2>&1; then
    fail "Prometheus returned a malformed or unsuccessful response for $endpoint."
  fi
  printf '%s' "$body"
}

# --- Precondition 1: durable, linked incident archive -----------------------
# Require an archived snapshot whose recorded active-marker hash matches the
# current marker and whose SHA256SUMS manifest verifies intact. A volatile API
# response, the marker alone, an empty directory, or a tampered archive is not
# durable evidence.
require_durable_archive() {
  [ -f "$incident_marker" ] || fail "The active incident marker is absent."
  [ -d "$archive_dir" ] || fail "No durable incident archive directory exists."

  marker_hash=$(shasum -a 256 "$incident_marker" | awk '{print $1}')
  [ -n "$marker_hash" ] || fail "The active incident marker could not be hashed."

  for candidate in "$archive_dir"/*/; do
    [ -d "$candidate" ] || continue
    [ -f "$candidate/SHA256SUMS" ] || continue
    link_file="$candidate/derived/active-incident.source.sha256"
    [ -f "$link_file" ] || continue
    recorded=$(awk 'NR==1{print $1}' "$link_file")
    [ "$recorded" = "$marker_hash" ] || continue
    if ( cd "$candidate" && shasum -a 256 -c SHA256SUMS >/dev/null 2>&1 ); then
      return 0
    fi
  done
  fail "No durable archived incident snapshot linked to the active marker was found."
}

require_durable_archive

# --- Precondition 2: exactly one healthy, inactive named rule ---------------
rules_json=$(fetch_success "/api/v1/rules")
rule_count=$(printf '%s' "$rules_json" | jq \
  --arg n "$alert_name" \
  '[.data.groups[]?.rules[]? | select(.type == "alerting" and .name == $n)] | length')
[ "$rule_count" = "1" ] \
  || fail "Expected exactly one $alert_name rule but found $rule_count."

if ! printf '%s' "$rules_json" | jq -e \
  --arg n "$alert_name" \
  '[.data.groups[]?.rules[]? | select(.type == "alerting" and .name == $n)][0]
     | (.health == "ok")
       and ((.lastError // "") == "")
       and (.state == "inactive")
       and ((.alerts // []) | length == 0)' >/dev/null 2>&1; then
  fail "The $alert_name rule is unhealthy, errored, or not inactive."
fi

# --- Precondition 3: no matching active alert -------------------------------
alerts_json=$(fetch_success "/api/v1/alerts")
active_matches=$(printf '%s' "$alerts_json" | jq \
  --arg n "$alert_name" \
  '[.data.alerts[]? | select(.labels.alertname == $n)] | length')
[ "$active_matches" = "0" ] \
  || fail "A matching active $alert_name alert is still present."

# --- Precondition 4: exactly one target for the job, and it is up -----------
targets_json=$(fetch_success "/api/v1/targets")
target_count=$(printf '%s' "$targets_json" | jq \
  --arg j "$target_job" \
  '[.data.activeTargets[]? | select(.labels.job == $j)] | length')
[ "$target_count" = "1" ] \
  || fail "Expected exactly one $target_job target but found $target_count."
if ! printf '%s' "$targets_json" | jq -e \
  --arg j "$target_job" \
  '[.data.activeTargets[]? | select(.labels.job == $j)][0].health == "up"' \
  >/dev/null 2>&1; then
  fail "The $target_job target is not up."
fi

# --- Precondition 5: exact recovery query is a single numeric zero ----------
query_json=$(fetch_success "/api/v1/query" -G \
  --data-urlencode "query=$recovery_query")
if ! printf '%s' "$query_json" | jq -e '
  (.data.resultType == "vector")
  and ((.data.result | length) == 1)
  and ((.data.result[0].value[1]) | type == "string")
  and ((.data.result[0].value[1])
        | test("^-?[0-9]+(\\.[0-9]+)?([eE][-+]?[0-9]+)?$"))
  and ((.data.result[0].value[1] | tonumber) == 0)' >/dev/null 2>&1; then
  fail "The exact 502 recovery query did not return a single numeric zero."
fi

# --- All preconditions passed: remove only the mutable marker ---------------
rm -f "$incident_marker"
echo "Portfolio API recovery verified against durable evidence; active incident marker cleared."
