#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# Owner-only: immutable Hub metadata, private backup, exact app-only replacement.
if [[ -x bin/media-toolctl ]];then exec bin/media-toolctl --root "$PWD" update "${1:?immutable Docker Hub image required}";fi
exec go run ./cmd/media-toolctl --root "$PWD" update "${1:?immutable Docker Hub image required}"
