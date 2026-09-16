#!/usr/bin/env sh
set -eu

# Exercises an already-running Core v1 stack. This script is strictly read-only
# with respect to the stack: it never starts, stops, resets, or otherwise
# mutates any service. It proves the exact seeded portfolio body and that a
# single seeded request increments each approved insight counter by exactly one.

base_url="${BASE_URL:-http://localhost:8080}"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

baseline_metrics="$work_dir/baseline-metrics.txt"
post_metrics="$work_dir/post-metrics.txt"
response_file="$work_dir/portfolio.json"

concentration_message="AAPL represents 54% of portfolio value; concentration insights are generated above 50%."
cash_message="Cash represents 15% of portfolio value; cash-buffer status is informational at or above 10%."

# insight_counter prints the cumulative value of one insight counter series.
# The label block contains no spaces, so the metric name plus labels is the
# first whitespace field and the value is the second.
insight_counter() {
  awk -v pat="portfolio_insights_generated_total{type=\"$2\"}" '$1 == pat { print $2 }' "$1"
}

# insight_types prints the sorted set of insight type label values present.
insight_types() {
  sed -n 's/^portfolio_insights_generated_total{type="\([^"]*\)"}.*/\1/p' "$1" | sort
}

# Capture the insight counters before issuing exactly one seeded request.
curl --fail --silent --show-error "$base_url/metrics" >"$baseline_metrics"
base_concentration="$(insight_counter "$baseline_metrics" CONCENTRATION)"
base_cash="$(insight_counter "$baseline_metrics" CASH_BUFFER)"
: "${base_concentration:?baseline CONCENTRATION insight series missing}"
: "${base_cash:?baseline CASH_BUFFER insight series missing}"

# Exactly one real seeded request drives both the API and metric assertions.
curl --fail --silent --show-error \
  "$base_url/api/v1/customers/cust-1001/portfolio" >"$response_file"

# Exact API assertions: preserved legacy fields plus the complete ordered
# two-item insight array. Object equality enforces the exact five-key shape.
jq -e '.customerId == "cust-1001"' "$response_file" >/dev/null
jq -e '.totalMarketValue.amount == "34375.00" and .totalMarketValue.currency == "USD"' "$response_file" >/dev/null
jq -e '([.accounts[] | {(.accountId): .marketValue.amount}] | add) ==
  {"acct-brokerage-01": "21025.00", "acct-retirement-01": "13350.00"}' "$response_file" >/dev/null
jq -e '(.insights | length) == 2' "$response_file" >/dev/null
jq -e --arg message "$concentration_message" '.insights[0] == {
  type: "CONCENTRATION", severity: "WARN", message: $message, symbol: "AAPL", percentage: 54
}' "$response_file" >/dev/null
jq -e --arg message "$cash_message" '.insights[1] == {
  type: "CASH_BUFFER", severity: "INFO", message: $message, symbol: null, percentage: 15
}' "$response_file" >/dev/null

# Capture the insight counters after the single seeded request.
curl --fail --silent --show-error "$base_url/metrics" >"$post_metrics"
post_concentration="$(insight_counter "$post_metrics" CONCENTRATION)"
post_cash="$(insight_counter "$post_metrics" CASH_BUFFER)"
: "${post_concentration:?post-request CONCENTRATION insight series missing}"
: "${post_cash:?post-request CASH_BUFFER insight series missing}"

# The instrument exposes exactly the two approved type series and no other.
observed_types="$(insight_types "$post_metrics")"
expected_types="$(printf '%s\n%s\n' CASH_BUFFER CONCENTRATION)"
if [ "$observed_types" != "$expected_types" ]; then
  echo "unexpected insight type series present:" >&2
  echo "$observed_types" >&2
  exit 1
fi

# A single seeded request increments each approved series by exactly one.
[ "$((post_concentration - base_concentration))" -eq 1 ] ||
  { echo "CONCENTRATION delta = $((post_concentration - base_concentration)), want 1" >&2; exit 1; }
[ "$((post_cash - base_cash))" -eq 1 ] ||
  { echo "CASH_BUFFER delta = $((post_cash - base_cash)), want 1" >&2; exit 1; }

# Preserve the existing liveness and request-metric coverage.
curl --fail --silent --show-error "$base_url/health/live" >/dev/null
grep -q 'portfolio_http_requests_total' "$post_metrics"

echo "End-to-end insight and telemetry checks passed."
