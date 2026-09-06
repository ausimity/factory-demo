#!/usr/bin/env sh
set -eu

base_url="${BASE_URL:-http://localhost:8080}"
response_file="$(mktemp)"
trap 'rm -f "$response_file"' EXIT

curl --fail --silent --show-error \
  "$base_url/api/v1/customers/cust-1001/portfolio" >"$response_file"

grep -q '"customerId":"cust-1001"' "$response_file"
grep -q '"amount":"34375.00"' "$response_file"
grep -q '"currency":"USD"' "$response_file"

curl --fail --silent --show-error "$base_url/health/live" >/dev/null
curl --fail --silent --show-error "$base_url/metrics" | grep -q 'portfolio_http_requests_total'

echo "End-to-end baseline checks passed."
