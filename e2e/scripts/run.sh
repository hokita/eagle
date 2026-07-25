#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$E2E_DIR")"

exec firebase emulators:exec --project eagle-test --only auth,firestore "$SCRIPT_DIR/inner.sh"
