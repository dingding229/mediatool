#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "${MEDIA_TOOL_TEST_LOCKED:-}" != 1 ]];then exec python3 scripts/test-lock.py "$0" "$@";fi
# Deliberately replace the normal deployment database BEFORE migrating.
set -a; source .env; source .env.test; set +a
export DATABASE_URL="${TEST_DATABASE_URL:?An isolated TEST_DATABASE_URL is required}"
python3 - <<'PY_GUARD'
import os, urllib.parse
u=urllib.parse.urlsplit(os.environ['DATABASE_URL'])
if urllib.parse.unquote(u.path)!='/media_tool_test':
 raise SystemExit('Go integration tests may only use media_tool_test.')
redis=urllib.parse.urlsplit(os.environ['REDIS_URL'])
if redis.scheme not in ('redis','rediss'):
 raise SystemExit('A separate Redis test database is required.')
PY_GUARD
REDIS_URL="$(python3 -c 'import os,urllib.parse;u=urllib.parse.urlsplit(os.environ["REDIS_URL"]);print(urllib.parse.urlunsplit(u._replace(path="/15")))')"
export REDIS_URL
export TEST_REDIS_URL="$REDIS_URL"
# Packages only: do not allow callers to override -p 1 with flags.
for package in "$@"; do
 if [[ "$package" == -* ]]; then echo 'Pass package paths only, not test flags.' >&2; exit 1; fi
done
if [[ "$#" -eq 0 ]]; then set -- ./...; fi
go run ./cmd/migrate -test-only -action up -cleanup-test-plugins
exec go test -p 1 -race -count=1 "$@"
