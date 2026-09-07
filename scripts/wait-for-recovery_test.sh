#!/usr/bin/env sh
# Deterministic, fixture-backed shell tests for scripts/wait-for-recovery.sh.
#
# These tests never touch the live Prometheus instance, the real incident
# marker, or the real evidence archive. Every case runs entirely under a
# uniquely named temporary directory with an injected fixture-backed `curl`
# shim on PATH. A no-op `sleep` shim keeps any legacy retry loop instantaneous
# so the suite stays deterministic and free of orphaned pollers.
set -eu

REPO_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
HELPER=${HELPER:-"$REPO_ROOT/scripts/wait-for-recovery.sh"}

WORK=$(mktemp -d "${TMPDIR:-/tmp}/wait-for-recovery-test.XXXXXX")
cleanup() {
  # Only ever remove this suite's uniquely named temporary tree.
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

# --- fixture-backed command shims -------------------------------------------
mkdir -p "$WORK/bin"

cat >"$WORK/bin/curl" <<'SHIM'
#!/usr/bin/env sh
# Route by requested Prometheus endpoint and serve fixtures from
# $REC_FIXTURE_DIR. A "<key>.exit" file simulates a curl failure (network
# error or non-2xx under --fail); a "<key>.json" file is served as the body.
url=""
for a in "$@"; do
  case "$a" in
    http*) url="$a" ;;
  esac
done
case "$url" in
  */api/v1/rules) key="rules" ;;
  */api/v1/alerts) key="alerts" ;;
  */api/v1/targets) key="targets" ;;
  */api/v1/query) key="query" ;;
  *) key="unknown" ;;
esac
dir=${REC_FIXTURE_DIR:?REC_FIXTURE_DIR must be set}
if [ -f "$dir/$key.exit" ]; then
  exit "$(cat "$dir/$key.exit")"
fi
if [ -f "$dir/$key.json" ]; then
  cat "$dir/$key.json"
  exit 0
fi
exit 7
SHIM
chmod +x "$WORK/bin/curl"

cat >"$WORK/bin/sleep" <<'SHIM'
#!/usr/bin/env sh
exit 0
SHIM
chmod +x "$WORK/bin/sleep"

# --- helpers ----------------------------------------------------------------
RULE_NAME="PortfolioAPIHigh502Rate"

# Create a fresh case tree with a synthetic active marker. Echoes the case dir.
new_case() {
  case_dir="$WORK/$1"
  rm -rf "$case_dir"
  mkdir -p "$case_dir/.local/incidents" "$case_dir/fixtures"
  cat >"$case_dir/.local/incidents/active.json" <<EOF
{"alert":"$RULE_NAME","status":"firing","service":"portfolio-api"}
EOF
  printf '%s' "$case_dir"
}

# Create a durable archive under the case that is linked to the active marker
# by recording the marker's SHA-256 and carries a self-consistent SHA256SUMS.
make_linked_archive() {
  cd="$1"
  arc="$cd/.local/incidents/archive/20260907T014402Z-diagnosis"
  mkdir -p "$arc/derived" "$arc/raw"
  marker_hash=$(shasum -a 256 "$cd/.local/incidents/active.json" | awk '{print $1}')
  printf '%s  .local/incidents/active.json\n' "$marker_hash" \
    >"$arc/derived/active-incident.source.sha256"
  cp "$cd/.local/incidents/active.json" "$arc/raw/active-incident.json"
  ( cd "$arc" && shasum -a 256 \
      derived/active-incident.source.sha256 \
      raw/active-incident.json >SHA256SUMS )
}

# Healthy fixture set: recovered, verified state.
write_healthy_fixtures() {
  d="$1/fixtures"
  cat >"$d/rules.json" <<EOF
{"status":"success","data":{"groups":[{"name":"portfolio-api","rules":[
{"type":"recording","name":"other","state":"inactive","health":"ok"},
{"type":"alerting","name":"$RULE_NAME","state":"inactive","health":"ok","lastError":"","alerts":[]}
]}]}}
EOF
  cat >"$d/alerts.json" <<'EOF'
{"status":"success","data":{"alerts":[]}}
EOF
  cat >"$d/targets.json" <<'EOF'
{"status":"success","data":{"activeTargets":[
{"labels":{"job":"portfolio-api","instance":"portfolio-api:8080"},"health":"up"}
]}}
EOF
  cat >"$d/query.json" <<'EOF'
{"status":"success","data":{"resultType":"vector","result":[
{"metric":{},"value":[1788745477.808,"0"]}
]}}
EOF
}

# --- assertions & runner ----------------------------------------------------
PASS=0
FAIL=0

run_helper() {
  cd="$1"
  ( cd "$cd" \
      && REC_FIXTURE_DIR="$cd/fixtures" \
         PROMETHEUS_URL="http://prom" \
         PATH="$WORK/bin:$PATH" \
         sh "$HELPER" >"$cd/out.log" 2>&1 )
}

archive_fingerprint() {
  cd="$1"
  if [ -d "$cd/.local/incidents/archive" ]; then
    find "$cd/.local/incidents/archive" -type f -exec shasum -a 256 {} \; \
      | sort
  fi
}

# expect_keep <name> <case_dir>: helper must fail closed (nonzero), keep the
# marker, and leave every archive byte unchanged.
expect_keep() {
  name="$1"
  cd="$2"
  before=$(archive_fingerprint "$cd")
  if run_helper "$cd"; then rc=0; else rc=$?; fi
  after=$(archive_fingerprint "$cd")
  if [ "$rc" -ne 0 ] \
     && [ -f "$cd/.local/incidents/active.json" ] \
     && [ "$before" = "$after" ]; then
    PASS=$((PASS + 1))
    printf 'PASS %s\n' "$name"
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL %s (rc=%s marker_present=%s archive_intact=%s)\n' \
      "$name" "$rc" \
      "$([ -f "$cd/.local/incidents/active.json" ] && echo yes || echo no)" \
      "$([ "$before" = "$after" ] && echo yes || echo no)"
  fi
}

# expect_remove <name> <case_dir>: helper must succeed (zero), remove exactly
# the marker, and leave every archive byte unchanged.
expect_remove() {
  name="$1"
  cd="$2"
  before=$(archive_fingerprint "$cd")
  if run_helper "$cd"; then rc=0; else rc=$?; fi
  after=$(archive_fingerprint "$cd")
  if [ "$rc" -eq 0 ] \
     && [ ! -e "$cd/.local/incidents/active.json" ] \
     && [ "$before" = "$after" ]; then
    PASS=$((PASS + 1))
    printf 'PASS %s\n' "$name"
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL %s (rc=%s marker_absent=%s archive_intact=%s)\n' \
      "$name" "$rc" \
      "$([ ! -e "$cd/.local/incidents/active.json" ] && echo yes || echo no)" \
      "$([ "$before" = "$after" ] && echo yes || echo no)"
  fi
}

# ============================================================================
# Success path
# ============================================================================
test_verified_recovery_removes_only_active_marker() {
  c=$(new_case verified); make_linked_archive "$c"; write_healthy_fixtures "$c"
  expect_remove "test_verified_recovery_removes_only_active_marker" "$c"
}

# ============================================================================
# Durable-archive linkage (evidence-preserving)
# ============================================================================
test_missing_archive_keeps_active_marker() {
  c=$(new_case no_archive); write_healthy_fixtures "$c"
  expect_keep "test_missing_archive_keeps_active_marker" "$c"
}

test_unlinked_archive_keeps_active_marker() {
  c=$(new_case unlinked_archive); make_linked_archive "$c"
  # Break linkage: record a hash that does not match the marker.
  printf '%s  .local/incidents/active.json\n' \
    "0000000000000000000000000000000000000000000000000000000000000000" \
    >"$c/.local/incidents/archive/20260907T014402Z-diagnosis/derived/active-incident.source.sha256"
  ( cd "$c/.local/incidents/archive/20260907T014402Z-diagnosis" \
      && shasum -a 256 derived/active-incident.source.sha256 \
           raw/active-incident.json >SHA256SUMS )
  write_healthy_fixtures "$c"
  expect_keep "test_unlinked_archive_keeps_active_marker" "$c"
}

test_incomplete_archive_manifest_keeps_active_marker() {
  c=$(new_case incomplete_archive); make_linked_archive "$c"
  # Corrupt a byte of an archived file so SHA256SUMS verification fails.
  printf 'tampered\n' >>"$c/.local/incidents/archive/20260907T014402Z-diagnosis/raw/active-incident.json"
  write_healthy_fixtures "$c"
  expect_keep "test_incomplete_archive_manifest_keeps_active_marker" "$c"
}

# ============================================================================
# Prometheus availability / transport
# ============================================================================
test_prometheus_unavailable_keeps_active_marker() {
  c=$(new_case prom_down); make_linked_archive "$c"; write_healthy_fixtures "$c"
  echo 7 >"$c/fixtures/rules.exit"
  expect_keep "test_prometheus_unavailable_keeps_active_marker" "$c"
}

test_non2xx_response_keeps_active_marker() {
  c=$(new_case prom_5xx); make_linked_archive "$c"; write_healthy_fixtures "$c"
  echo 22 >"$c/fixtures/query.exit"
  expect_keep "test_non2xx_response_keeps_active_marker" "$c"
}

test_malformed_json_keeps_active_marker() {
  c=$(new_case malformed); make_linked_archive "$c"; write_healthy_fixtures "$c"
  printf 'this is not json{' >"$c/fixtures/rules.json"
  expect_keep "test_malformed_json_keeps_active_marker" "$c"
}

# ============================================================================
# Named rule health / cardinality / state
# ============================================================================
test_absent_rule_keeps_active_marker() {
  c=$(new_case rule_absent); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/rules.json" <<'EOF'
{"status":"success","data":{"groups":[{"name":"portfolio-api","rules":[]}]}}
EOF
  expect_keep "test_absent_rule_keeps_active_marker" "$c"
}

test_duplicate_rule_keeps_active_marker() {
  c=$(new_case rule_dup); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/rules.json" <<EOF
{"status":"success","data":{"groups":[{"name":"portfolio-api","rules":[
{"type":"alerting","name":"$RULE_NAME","state":"inactive","health":"ok","lastError":"","alerts":[]},
{"type":"alerting","name":"$RULE_NAME","state":"inactive","health":"ok","lastError":"","alerts":[]}
]}]}}
EOF
  expect_keep "test_duplicate_rule_keeps_active_marker" "$c"
}

test_unhealthy_rule_keeps_active_marker() {
  c=$(new_case rule_unhealthy); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/rules.json" <<EOF
{"status":"success","data":{"groups":[{"name":"portfolio-api","rules":[
{"type":"alerting","name":"$RULE_NAME","state":"inactive","health":"err","lastError":"boom","alerts":[]}
]}]}}
EOF
  expect_keep "test_unhealthy_rule_keeps_active_marker" "$c"
}

test_non_inactive_rule_keeps_active_marker() {
  c=$(new_case rule_firing); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/rules.json" <<EOF
{"status":"success","data":{"groups":[{"name":"portfolio-api","rules":[
{"type":"alerting","name":"$RULE_NAME","state":"firing","health":"ok","lastError":"","alerts":[{"state":"firing"}]}
]}]}}
EOF
  expect_keep "test_non_inactive_rule_keeps_active_marker" "$c"
}

# ============================================================================
# Exact recovery query
# ============================================================================
test_empty_query_keeps_active_marker() {
  c=$(new_case query_empty); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/query.json" <<'EOF'
{"status":"success","data":{"resultType":"vector","result":[]}}
EOF
  expect_keep "test_empty_query_keeps_active_marker" "$c"
}

test_duplicate_query_result_keeps_active_marker() {
  c=$(new_case query_dup); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/query.json" <<'EOF'
{"status":"success","data":{"resultType":"vector","result":[
{"metric":{"status":"502"},"value":[1788745477.808,"0"]},
{"metric":{"status":"502"},"value":[1788745477.808,"0"]}
]}}
EOF
  expect_keep "test_duplicate_query_result_keeps_active_marker" "$c"
}

test_nonzero_query_keeps_active_marker() {
  c=$(new_case query_nonzero); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/query.json" <<'EOF'
{"status":"success","data":{"resultType":"vector","result":[
{"metric":{},"value":[1788745477.808,"3.5"]}
]}}
EOF
  expect_keep "test_nonzero_query_keeps_active_marker" "$c"
}

test_nonnumeric_query_keeps_active_marker() {
  c=$(new_case query_nonnumeric); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/query.json" <<'EOF'
{"status":"success","data":{"resultType":"vector","result":[
{"metric":{},"value":[1788745477.808,"NaN"]}
]}}
EOF
  expect_keep "test_nonnumeric_query_keeps_active_marker" "$c"
}

# ============================================================================
# Target health / cardinality
# ============================================================================
test_absent_target_keeps_active_marker() {
  c=$(new_case target_absent); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/targets.json" <<'EOF'
{"status":"success","data":{"activeTargets":[]}}
EOF
  expect_keep "test_absent_target_keeps_active_marker" "$c"
}

test_duplicate_target_keeps_active_marker() {
  c=$(new_case target_dup); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/targets.json" <<'EOF'
{"status":"success","data":{"activeTargets":[
{"labels":{"job":"portfolio-api"},"health":"up"},
{"labels":{"job":"portfolio-api"},"health":"up"}
]}}
EOF
  expect_keep "test_duplicate_target_keeps_active_marker" "$c"
}

test_down_target_keeps_active_marker() {
  c=$(new_case target_down); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/targets.json" <<'EOF'
{"status":"success","data":{"activeTargets":[
{"labels":{"job":"portfolio-api"},"health":"down"}
]}}
EOF
  expect_keep "test_down_target_keeps_active_marker" "$c"
}

# ============================================================================
# Active alert
# ============================================================================
test_active_alert_keeps_active_marker() {
  c=$(new_case active_alert); make_linked_archive "$c"; write_healthy_fixtures "$c"
  cat >"$c/fixtures/alerts.json" <<EOF
{"status":"success","data":{"alerts":[
{"labels":{"alertname":"$RULE_NAME","service":"portfolio-api"},"state":"firing"}
]}}
EOF
  expect_keep "test_active_alert_keeps_active_marker" "$c"
}

# ============================================================================
main() {
  printf 'Running wait-for-recovery shell tests against: %s\n' "$HELPER"
  test_verified_recovery_removes_only_active_marker
  test_missing_archive_keeps_active_marker
  test_unlinked_archive_keeps_active_marker
  test_incomplete_archive_manifest_keeps_active_marker
  test_prometheus_unavailable_keeps_active_marker
  test_non2xx_response_keeps_active_marker
  test_malformed_json_keeps_active_marker
  test_absent_rule_keeps_active_marker
  test_duplicate_rule_keeps_active_marker
  test_unhealthy_rule_keeps_active_marker
  test_non_inactive_rule_keeps_active_marker
  test_empty_query_keeps_active_marker
  test_duplicate_query_result_keeps_active_marker
  test_nonzero_query_keeps_active_marker
  test_nonnumeric_query_keeps_active_marker
  test_absent_target_keeps_active_marker
  test_duplicate_target_keeps_active_marker
  test_down_target_keeps_active_marker
  test_active_alert_keeps_active_marker
  printf '\n%s passed, %s failed\n' "$PASS" "$FAIL"
  [ "$FAIL" -eq 0 ]
}

main "$@"
