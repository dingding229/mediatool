#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/test-common.sh
isolated_test "$@"
pattern='^TestRealHelperEntrypointDocker$'
if [[ "$#" -eq 1 && "$1" == startup-config ]];then pattern='^TestRealHelperEntrypointDocker/(image_config_.*|success)$'
elif [[ "$#" -ne 0 ]];then echo 'Only the optional startup-config directed mode is supported.' >&2;exit 1;fi
# Keep the single full race pipeline unchanged. This opt-in case needs Docker
# access (the full suite's plugin isolation UID deliberately does not have it).
go run ./cmd/media-tool migrate -test-only -action up -cleanup-test-plugins
export MEDIA_TOOL_TEST_HELPER_DOCKER=1
go test -race -p 1 -count=1 -v ./internal/modules/onlineupdate -run "$pattern"
