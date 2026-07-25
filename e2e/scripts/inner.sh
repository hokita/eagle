#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$E2E_DIR")"

echo "Clearing Firestore emulator data..."
curl -s -X DELETE "http://${FIRESTORE_EMULATOR_HOST}/emulator/v1/projects/eagle-test/databases/(default)/documents" > /dev/null

echo "Seeding fixture sentences..."
(cd "$REPO_ROOT/api" && GOOGLE_CLOUD_PROJECT=eagle-test go run ./cmd/seed -file "$E2E_DIR/fixtures/sentences.ndjson")

echo "Running Playwright..."
(cd "$E2E_DIR" && npx playwright test)
