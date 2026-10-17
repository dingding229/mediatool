#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# Share the database-mutating suite lock. Never run in parallel with E2E/Go DB tests.
if [[ "${MEDIA_TOOL_TEST_LOCKED:-}" != 1 ]]; then
  exec python3 scripts/test-lock.py "$0" "$@"
fi
if [[ "$#" != 0 ]]; then echo 'No arguments accepted.' >&2; exit 1; fi
# Only the dedicated test DSN is loaded, never the initialized production .env.
if [[ -z "${TEST_DATABASE_URL:-}" && -f .env.test ]]; then
  set -a; source .env.test; set +a
fi
export DATABASE_URL="${TEST_DATABASE_URL:?An isolated TEST_DATABASE_URL is required}"
python3 - <<'PY'
import os, urllib.parse
u=urllib.parse.urlsplit(os.environ['DATABASE_URL'])
if u.scheme not in ('postgres','postgresql') or urllib.parse.unquote(u.path)!='/media_tool_test':
    raise SystemExit('Native Emby smoke may only use media_tool_test.')
PY
# No Redis or remote service is needed by this smoke test.
unset REDIS_URL TEST_REDIS_URL
go run ./cmd/migrate -test-only -action up
go test -p 1 -race -count=1 -v ./internal/modules/settings ./internal/platform/httpapi \
  -run '^(TestNativeServerCache.*|TestNativeListenerDatabaseSmoke)$'
