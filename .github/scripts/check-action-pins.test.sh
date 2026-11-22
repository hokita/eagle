#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
checker="$here/check-action-pins.sh"
fixtures="$here/fixtures/pins"

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

set +e
good_out="$(bash "$checker" "$fixtures/good" 2>&1)"
good_code=$?
bad_out="$(bash "$checker" "$fixtures/bad" 2>&1)"
bad_code=$?
set -e

[ "$good_code" -eq 0 ] || fail "good fixtures exited $good_code: $good_out"
[ -z "$good_out" ] || fail "good fixtures printed output: $good_out"
[ "$bad_code" -eq 1 ] || fail "bad fixtures exited $bad_code, want 1"

count="$(printf '%s\n' "$bad_out" | grep -c ':[0-9][0-9]*: ')"
[ "$count" -eq 4 ] || fail "bad fixtures reported $count offenders, want 4: $bad_out"

echo "PASS"
