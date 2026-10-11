#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
npm ci --prefix web --no-fund
# Clean only generated Vite JS/CSS outputs; preserve the embed sentinel.
python3 - <<'CLEAN'
from pathlib import Path
assets = Path('internal/platform/webui/assets')
for name in ('assets/*.js', 'assets/*.css', 'index.html'):
 for file in assets.glob(name):
  if file.is_file() and not file.is_symlink():
   file.unlink()
CLEAN
npm run build --prefix web
