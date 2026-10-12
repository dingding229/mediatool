#!/usr/bin/env python3
"""Build a deterministic ZIP from a WASM plugin directory (never execute it)."""
import argparse
from pathlib import Path
from zipfile import ZipFile, ZipInfo, ZIP_DEFLATED

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('source', type=Path)
parser.add_argument('output', type=Path)
args = parser.parse_args()
allowed = {'manifest.json', 'settings.json', 'README.md', 'plugin.wasm', 'ui.html'}
files = sorted(args.source.iterdir())
if not files or any(p.is_symlink() or not p.is_file() or p.name not in allowed for p in files):
    parser.error('Only supported root manifest/settings/README/WASM/UI regular files are supported')
if not (args.source / 'manifest.json').is_file() or not (args.source / 'plugin.wasm').is_file():
    parser.error('manifest.json and executable plugin.wasm required')
args.output.parent.mkdir(parents=True, exist_ok=True)
with ZipFile(args.output, 'w', compression=ZIP_DEFLATED) as archive:
    for p in files:
        entry = ZipInfo(p.name, (2026, 10, 12, 0, 0, 0))
        entry.compress_type = ZIP_DEFLATED
        entry.external_attr = 0o100644 << 16
        archive.writestr(entry, p.read_bytes())
print(args.output)
