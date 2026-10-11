#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ ! -f .env ]]; then echo 'Run python3 scripts/init-env.py first.' >&2; exit 1; fi
# .env is a trusted local configuration. Never source an untrusted upload.
set -a
source .env
set +a
export APP_PUBLIC_URL=http://localhost:5173
export APP_ENV=development
export APP_ADDR=127.0.0.1:8080
if [[ ! -d web/node_modules ]]; then npm ci --prefix web --no-fund; fi
go build -o bin/media-tool ./cmd/media-tool
./bin/media-tool &
backend_pid=$!
cleanup() { kill "$backend_pid" 2>/dev/null || true; wait "$backend_pid" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
npm run dev --prefix web
