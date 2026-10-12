#!/usr/bin/env python3
"""Build the runnable Go/WASI example and update its deterministic public catalog."""
import os,subprocess,tempfile,json,hashlib
from pathlib import Path
from zipfile import ZipFile,ZipInfo,ZIP_DEFLATED
root=Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory()as directory:
    module=Path(directory)/'plugin.wasm';env=dict(os.environ,GOOS='wasip1',GOARCH='wasm',CGO_ENABLED='0')
    subprocess.run(['go','build','-buildvcs=false','-trimpath','-ldflags=-s -w','-o',str(module),'./examples/plugins/automation'],cwd=root,env=env,check=True)
    source=root/'examples/plugins/automation';target=root/'examples/plugins/automation.zip'
    with ZipFile(target,'w',compression=ZIP_DEFLATED)as z:
        for name,data in [(name,(source/name).read_bytes())for name in ['manifest.json','README.md','ui.html']]+[('plugin.wasm',module.read_bytes())]:
            info=ZipInfo(name,(2026,10,12,0,0,0));info.compress_type=ZIP_DEFLATED;info.external_attr=0o100644<<16;z.writestr(info,data)
    manifest=json.loads((source/'manifest.json').read_text());entry={k:manifest.get(k,'')for k in ['id','name','version','author','description']};entry.update(packageUrl='automation.zip',digest=hashlib.sha256(target.read_bytes()).hexdigest())
    (root/'examples/plugins/index.json').write_text(json.dumps({'contractVersion':1,'plugins':[entry]},ensure_ascii=False,indent=2)+'\n')
print('Deterministic Go/WASI example ZIP/catalog built; no host credentials included.')
