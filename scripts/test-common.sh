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

# Reuse only this CI run's explicitly built, same-commit application. This is
# not a cross-run cache or a substitute for production Docker Hub publication.
copy_test_app(){
 local source="${MEDIA_TOOL_STATIC_TEST_BINARY:-}" revision
 [[ "${GITHUB_ACTIONS:-}" == true && -n "${GITHUB_SHA:-}" && -x "$source" && -f "$source.sha" ]] || { echo 'Same-commit CI binary required';return 1; }
 read -r revision < "$source.sha"
 [[ "$revision" == "$GITHUB_SHA" ]] || { echo 'Stale CI binary refused';return 1; }
 cp "$source" "$1"
 chmod 755 "$1"
}
