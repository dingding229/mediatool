#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
npm ci --prefix web --no-fund
# Clean only generated Vite JS/CSS outputs; preserve the embed sentinel.
go run ./cmd/media-tool-dev clean-web
npm run build --prefix web
