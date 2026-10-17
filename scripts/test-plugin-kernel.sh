#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "${MEDIA_TOOL_TEST_LOCKED:-}" != 1 ]];then exec python3 scripts/test-lock.py "$0" "$@";fi
[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || { echo 'Native Linux AMD64 release gate required';exit 1; }
work=$(mktemp -d)
trap 'chmod -R u+w "$work"; rm -rf "$work"' EXIT
CGO_ENABLED=0 go build -trimpath -o "$work/media-tool" ./cmd/media-tool
CGO_ENABLED=0 go test -c -o "$work/sandbox.test" ./internal/modules/plugins/v2/sandbox
CGO_ENABLED=0 go test -c -o "$work/worker.test" ./internal/modules/plugins/v2
CGO_ENABLED=0 go test -c -o "$work/httpapi.test" ./internal/platform/httpapi
CGO_ENABLED=0 go test -c -o "$work/acquisition.test" ./internal/modules/acquisition
# Gate runs non-root, without capabilities, with the production no-new-privileges.
# DB fixture URLs are injected only into the host test, never workerEnvironment.
set -a;source .env;source .env.test;set +a
python3 - <<'PY'
import os,urllib.parse
if urllib.parse.urlsplit(os.environ['TEST_DATABASE_URL']).path!='/media_tool_test':raise SystemExit('isolated database only')
PY
chmod 755 "$work" "$work/"*
sudo setpriv --reuid=65532 --regid=65532 --clear-groups --bounding-set=-all --no-new-privs "$work/sandbox.test" -test.v
sudo env TEST_DATABASE_URL="$TEST_DATABASE_URL" MEDIA_TOOL_PLUGIN_BINARY="$work/media-tool" setpriv --reuid=65532 --regid=65532 --clear-groups --bounding-set=-all --no-new-privs "$work/worker.test" -test.v

sudo env TEST_DATABASE_URL="$TEST_DATABASE_URL" MEDIA_TOOL_PLUGIN_BINARY="$work/media-tool" setpriv --reuid=65532 --regid=65532 --clear-groups --bounding-set=-all --no-new-privs "$work/httpapi.test" -test.v -test.run "^TestPluginRealBinaryHTTPServicesTransferAndDisable$"

sudo env TEST_DATABASE_URL="$TEST_DATABASE_URL" MEDIA_TOOL_PLUGIN_BINARY="$work/media-tool" setpriv --reuid=65532 --regid=65532 --clear-groups --bounding-set=-all --no-new-privs "$work/acquisition.test" -test.v -test.run "^TestPluginRealBinaryDownloadAdded$"
