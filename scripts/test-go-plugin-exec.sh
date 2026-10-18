#!/usr/bin/env bash
# Test-only Go -exec adapter. Never shipped or used as a production launcher.
# Go strips a /v2 major suffix when naming the binary: plugins.test, not v2.test.
set -euo pipefail
case "${1##*/}" in
 plugins.test|v2.test|httpapi.test|acquisition.test|messaging.test) ;;
 *) exec "$@";;
esac
[[ "$(uname -s)" == Linux && "${GITHUB_ACTIONS:-}" == true && "${MEDIA_TOOL_TEST_PLUGIN_UID:-}" == 65532 ]] || { echo 'Dedicated native CI identity required' >&2;exit 1; }
root=${MEDIA_TOOL_TEST_UID_DIR:?}
case "$root" in /tmp/media-tool-test-worker.*) ;; *) echo 'Invalid test artifact root' >&2;exit 1;; esac
# -p 1 guarantees no package shares this executable while it is running.
install -m 755 "$1" "$root/native-host.test"
shift
# Only fixture configuration is retained; no GitHub token/runner credentials.
keep=TEST_DATABASE_URL,TEST_REDIS_URL,DATABASE_URL,REDIS_URL,APP_SECRET_KEY,SESSION_SECRET_KEY,STRM_API_KEY,TEST_DB_EXEC_MODE,TEST_POSTGRES_CONTAINER,MEDIA_TOOL_REQUIRE_FFMPEG,MEDIA_TOOL_PLUGIN_BINARY,MEDIA_TOOL_TEST_PLUGIN_UID,GOTOOLCHAIN
exec sudo --preserve-env="$keep" setpriv --reuid=65532 --regid=65532 --clear-groups --bounding-set=-all --no-new-privs env PATH="$PATH" HOME="$root/uid-home" TMPDIR="$root/uid-tmp" GOCACHE="$root/uid-cache" "$root/native-host.test" "$@"
