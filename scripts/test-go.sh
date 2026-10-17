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
# Exercise the actual CGO-free release worker, not a race-instrumented test
# executable under production RLIMIT_AS/RSS. The host test itself remains -race.
# This also makes the HTTP/downloader same-binary tests mandatory on Linux.
test_exec=()
if [[ "$(uname -s)" == Linux ]]; then
 worker_dir=$(mktemp -d "${TMPDIR:-/tmp}/media-tool-test-worker.XXXXXX")
 trap 'if [[ ${MEDIA_TOOL_TEST_UID_DIR:-} == "$worker_dir" ]];then sudo chown -R "$(id -u):$(id -g)" "$worker_dir";fi;chmod -R u+w "$worker_dir";rm -rf "$worker_dir"' EXIT
 CGO_ENABLED=0 go build -trimpath -o "$worker_dir/media-tool" ./cmd/media-tool
 export MEDIA_TOOL_PLUGIN_BINARY="$worker_dir/media-tool"
 if [[ "${GITHUB_ACTIONS:-}" == true && "${MEDIA_TOOL_TEST_PLUGIN_UID:-}" == 65532 ]];then
  # RLIMIT_NPROC counts all threads sharing a real UID, including the GitHub
  # runner/.NET processes. Use the production identity for plugin host tests.
  chmod 755 "$worker_dir"
  mkdir -p "$worker_dir/uid-home" "$worker_dir/uid-tmp" "$worker_dir/uid-cache"
  sudo chown 65532:65532 "$worker_dir/uid-home" "$worker_dir/uid-tmp" "$worker_dir/uid-cache"
  export MEDIA_TOOL_TEST_UID_DIR="$worker_dir"
  test_exec=(-exec "$PWD/scripts/test-go-plugin-exec.sh")
  echo 'Plugin host packages use UID 65532 with cap-drop ALL/no-new-privileges; NPROC remains 64.'
 fi
 echo 'Real static app plugin-worker REQUIRED on Linux (host tests still use -race).'
fi
if [[ "${MEDIA_TOOL_REQUIRE_FFMPEG:-}" == 1 ]]; then
 command -v ffmpeg >/dev/null && command -v ffprobe >/dev/null
 echo 'Real media tests REQUIRED (missing binary/encoder is a failure); verbose output enabled.'
 go test "${test_exec[@]}" -p 1 -race -count=1 -v "$@"
 exit "$?"
fi
go test "${test_exec[@]}" -p 1 -race -count=1 "$@"
