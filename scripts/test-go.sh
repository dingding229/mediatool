#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/test-common.sh
isolated_test "$@"
# Packages only: do not allow callers to override -p 1 with flags.
for package in "$@"; do
 if [[ "$package" == -* ]]; then echo 'Pass package paths only, not test flags.' >&2; exit 1; fi
done
if [[ "$#" -eq 0 ]]; then set -- ./...; fi
go run ./cmd/media-tool migrate -test-only -action up -cleanup-test-plugins
# Exercise the actual CGO-free release worker, not a race-instrumented test
# executable under production RLIMIT_AS/RSS. The host test itself remains -race.
# This also makes the HTTP/downloader same-binary tests mandatory on Linux.
test_exec=(go test)
if [[ "$(uname -s)" == Linux ]]; then
 worker_dir=$(mktemp -d "${TMPDIR:-/tmp}/media-tool-test-worker.XXXXXX")
 trap 'if [[ ${MEDIA_TOOL_TEST_UID_DIR:-} == "$worker_dir" ]];then sudo chown -R "$(id -u):$(id -g)" "$worker_dir";fi;chmod -R u+w "$worker_dir";rm -rf "$worker_dir"' EXIT
 CGO_ENABLED=0 go build -trimpath -o "$worker_dir/media-tool" ./cmd/media-tool
 export MEDIA_TOOL_PLUGIN_BINARY="$worker_dir/media-tool"
 if [[ "${GITHUB_ACTIONS:-}" == true && "${MEDIA_TOOL_TEST_PLUGIN_UID:-}" == 65532 ]];then
  # Keep production UID/capabilities for isolation evidence. Task budgets are
  # per worker tree, independent of other processes sharing this UID.
  chmod 755 "$worker_dir"
  mkdir -p "$worker_dir/uid-home" "$worker_dir/uid-tmp" "$worker_dir/uid-cache"
  sudo chown 65532:65532 "$worker_dir/uid-home" "$worker_dir/uid-tmp" "$worker_dir/uid-cache"
  export MEDIA_TOOL_TEST_UID_DIR="$worker_dir"
  test_exec+=(-exec "$PWD/scripts/test-go-plugin-exec.sh")
  echo 'Plugin host packages use UID 65532 with cap-drop ALL/no-new-privileges; per-worker task watchdog enabled (no UID-wide NPROC).'
 fi
 echo 'Real static app plugin-worker REQUIRED on Linux (host tests still use -race).'
fi
# The growing migration/compatibility package can exceed Go's implicit 10m
# package alarm on local PostgreSQL. Keep it bounded; no individual deadline,
# worker resource limit, race flag or assertion is relaxed.
if [[ "${MEDIA_TOOL_REQUIRE_FFMPEG:-}" == 1 ]]; then
 command -v ffmpeg >/dev/null && command -v ffprobe >/dev/null
 echo 'Real media tests REQUIRED (missing binary/encoder is a failure); verbose output enabled.'
 "${test_exec[@]}" -p 1 -race -count=1 -v -timeout=20m "$@"
 exit "$?"
fi
"${test_exec[@]}" -p 1 -race -count=1 -timeout=20m "$@"
