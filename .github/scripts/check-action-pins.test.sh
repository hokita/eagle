#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
checker="$here/check-action-pins.sh"
fixtures="$here/fixtures/pins"
sha=0123456789abcdef0123456789abcdef01234567

work="$(mktemp -d)"
trap 'chmod -R u+rwx "$work"; rm -rf "$work"' EXIT

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

run() {
  set +e
  out="$(bash "$checker" "$@" 2>&1)"
  code=$?
  set -e
}

run "$fixtures/good"
[ "$code" -eq 0 ] || fail "good fixtures exited $code: $out"
[ -z "$out" ] || fail "good fixtures printed output: $out"

run "$fixtures/bad"
[ "$code" -eq 1 ] || fail "bad fixtures exited $code, want 1"
bad="$fixtures/bad/a.yml"
expected="$bad:5: actions/checkout@v4
$bad:6: actions/setup-go@main
$bad:8: actions/cache@0123456
$bad:9: docker://alpine:3
$bad:11: actions/upper@0123456789ABCDEF0123456789abcdef01234567
$bad:12: actions/short@${sha%?}
$bad:13: actions/long@${sha}8
$bad:14: docker://x@$sha
$bad:15: foo@$sha"
[ "$out" = "$expected" ] || fail "bad fixtures output mismatch:
$out
--- want ---
$expected"

run
[ "$code" -eq 2 ] || fail "missing argument exited $code, want 2"

run "$work/does-not-exist"
[ "$code" -eq 2 ] || fail "nonexistent dir exited $code, want 2"

mkdir "$work/empty"
run "$work/empty"
[ "$code" -eq 2 ] || fail "empty dir exited $code, want 2"

mkdir "$work/unreadable"
printf 'steps:\n  - uses: actions/checkout@%s\n' "$sha" >"$work/unreadable/a.yml"
chmod 000 "$work/unreadable/a.yml"
run "$work/unreadable"
[ "$code" -ne 0 ] || fail "unreadable file exited 0"

echo "PASS"
