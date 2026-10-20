#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ ! -f .env ]]; then echo 'Run go run ./cmd/media-tool-dev init-env first.' >&2; exit 1; fi
set -a; source .env; set +a
exec "$@"
