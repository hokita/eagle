#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: check-action-pins.sh <workflows-dir>" >&2
  exit 2
}

[ "$#" -eq 1 ] || usage
dir="$1"
[ -d "$dir" ] || usage

shopt -s nullglob
files=("$dir"/*.yml "$dir"/*.yaml)
if [ "${#files[@]}" -eq 0 ]; then
  echo "check-action-pins.sh: no .yml or .yaml files in $dir" >&2
  exit 2
fi

sha_re='^[^@/:[:space:]]+/[^@/:[:space:]]+(/[^@[:space:]]+)?@[0-9a-f]{40}$'
docker_re='^docker://[^@[:space:]]+@sha256:[0-9a-f]{64}$'
local_re='^\./'

status=0
for file in "${files[@]}"; do
  grep_status=0
  hits="$(grep -nE '^[[:space:]]*(-[[:space:]]+)?uses:' "$file")" || grep_status=$?
  [ "$grep_status" -le 1 ] || exit 2
  [ -n "$hits" ] || continue
  while IFS= read -r hit; do
    line="${hit%%:*}"
    value="${hit#*:}"
    value="$(printf '%s' "$value" | sed -E 's/^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*//; s/[[:space:]]+#.*$//; s/[[:space:]]+$//')"
    value="${value#\'}"
    value="${value%\'}"
    value="${value#\"}"
    value="${value%\"}"
    if printf '%s' "$value" | grep -qE "$sha_re|$docker_re|$local_re"; then
      continue
    fi
    echo "$file:$line: $value"
    status=1
  done <<<"$hits"
done
exit "$status"
