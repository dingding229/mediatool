#!/usr/bin/env python3
"""Generate separate local session/playback keys; never overwrite .env."""
import os,secrets
from pathlib import Path
root=Path(__file__).resolve().parent.parent
values={k:secrets.token_hex(32) for k in ('SESSION_SECRET_KEY','STRM_API_KEY','POSTGRES_PASSWORD','REDIS_PASSWORD','APP_SECRET_KEY')}
text=(root/'.env.example').read_text()
for k,v in values.items():text=text.replace(k+'=\n',k+'='+v+'\n')
try:
 fd=os.open(root/'.env',os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
except FileExistsError:
 raise SystemExit('.env already exists; leaving all existing credentials unchanged.')
with os.fdopen(fd,'w') as f:f.write(text)
(root/'data'/'strm').mkdir(parents=True,exist_ok=True)
print('Created .env with mode 0600. Keys are not printed. Open the local management UI to register the first administrator account.')
