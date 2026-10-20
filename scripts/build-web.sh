#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go run ./cmd/media-tool-dev plugins check
go run ./cmd/media-tool-dev plugins assets --check
go run ./cmd/media-tool-dev plugins assets
npm ci --prefix web --no-fund --no-audit
# Clean only generated Vite JS/CSS outputs; preserve the embed sentinel.
go run ./cmd/media-tool-dev clean-web
npm run build --prefix web
