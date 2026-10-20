#!/usr/bin/env bash
# One fail-fast command, one shared lock, no production database/environment.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/test-common.sh
isolated_test "$@"
export MEDIA_TOOL_REQUIRE_FFMPEG=1
ERRCHECK_BIN=${ERRCHECK_BIN:-errcheck}
GOVULNCHECK_BIN=${GOVULNCHECK_BIN:-govulncheck}
command -v "$ERRCHECK_BIN" >/dev/null
command -v "$GOVULNCHECK_BIN" >/dev/null
run(){ printf '\n===== %s =====\n' "$*"; /usr/bin/time -p "$@"; }
run bash scripts/build-web.sh
run go build ./...
run go vet ./...
run bash scripts/test-go.sh
run "$ERRCHECK_BIN" -ignoretests ./cmd/... ./internal/... ./migrations/... ./tests/...
run npm test --prefix web
run go run ./cmd/media-tool-dev plugins check
run go run ./cmd/media-tool-dev plugins assets --check
run go run ./cmd/media-tool-dev plugins matrix --check
run bash scripts/test-e2e.sh
run "$GOVULNCHECK_BIN" ./...
printf '\nCOMPLETE_GATE_EXIT_0\n'
