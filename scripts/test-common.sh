#!/usr/bin/env bash
# Shared runner: all DB-mutating suites use the same lock and isolated values.
devtool(){
 if [[ -n "${MEDIA_TOOL_DEV_BIN:-}" ]];then "$MEDIA_TOOL_DEV_BIN" "$@";else go run ./cmd/media-tool-dev "$@";fi
}
isolated_test(){
 if [[ "${MEDIA_TOOL_TEST_LOCKED:-}" != 1 ]];then devtool test-lock bash "$0" "$@";exit "$?";fi
 local values
 values=$(devtool test-env) || return "$?"
 eval "$values"
}
