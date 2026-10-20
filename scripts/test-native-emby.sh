#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/test-common.sh
isolated_test "$@"
if [[ "$#" != 0 ]];then echo "No arguments accepted." >&2;exit 1;fi
# No Redis or remote service is needed by this smoke test.
unset REDIS_URL TEST_REDIS_URL
go run ./cmd/media-tool migrate -test-only -action up
go test -p 1 -race -count=1 -v ./internal/modules/settings ./internal/platform/httpapi \
  -run '^(TestNativeServerCache.*|TestNativeListenerDatabaseSmoke)$'
