#!/usr/bin/env bash
set -euo pipefail

dir="${1:?usage: check-action-pins.sh <workflows-dir>}"

sha_re='^[^@[:space:]]+@[0-9a-f]{40}$'
docker_re='^docker://[^[:space:]]+@sha256:[0-9a-f]{64}$'
local_re='^\./'

status=0
for file in "$dir"/*.yml "$dir"/*.yaml; do
  [ -f "$file" ] || continue
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
  done < <(grep -nE '^[[:space:]]*(-[[:space:]]+)?uses:' "$file" || true)
done
exit "$status"
