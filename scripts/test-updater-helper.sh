#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/test-common.sh
isolated_test "$@"
[[ "$#" -eq 0 ]] || { echo 'This runner only executes the isolated updater entrypoint.' >&2; exit 1; }
# Keep the single full race pipeline unchanged. This opt-in case needs Docker
# access (the full suite's plugin isolation UID deliberately does not have it).
go run ./cmd/media-tool migrate -test-only -action up -cleanup-test-plugins
export MEDIA_TOOL_TEST_HELPER_DOCKER=1
go test -race -p 1 -count=1 -v ./internal/modules/onlineupdate -run '^TestRealHelperEntrypointDocker$'
