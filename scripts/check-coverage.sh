#!/usr/bin/env sh
set -eu

profile=${1:-artifacts/coverage.out}
minimum=${2:-50}

if [ ! -s "$profile" ]; then
  echo "Coverage profile is missing or empty: $profile" >&2
  exit 1
fi

actual=$(go tool cover -func="$profile" | awk '$1 == "total:" {gsub("%", "", $3); print $3}')
if [ -z "$actual" ]; then
  echo "Could not read total coverage from $profile" >&2
  exit 1
fi

if ! awk -v actual="$actual" -v minimum="$minimum" 'BEGIN {exit !(actual >= minimum)}'; then
  echo "Total coverage ${actual}% is below the required ${minimum}%." >&2
  exit 1
fi

echo "Total coverage ${actual}% meets the required ${minimum}%."
