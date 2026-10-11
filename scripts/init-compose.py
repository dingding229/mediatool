#!/usr/bin/env python3
"""Initialize only placeholder credentials in the ONE operator-owned Compose.
No .env is needed for deployment. Never print generated credentials or change
already configured keys/database passwords. Do not commit the initialized file.
"""
import os,secrets,tempfile
from pathlib import Path
root=Path(__file__).resolve().parent.parent
p=root/'docker-compose.yml'
text=p.read_text()
keys=['APP_SECRET_KEY','SESSION_SECRET_KEY','STRM_API_KEY','POSTGRES_PASSWORD','REDIS_PASSWORD']
changed=False
for key in keys:
 placeholder='CHANGE_ME_'+key
 if placeholder in text:
  text=text.replace(placeholder,secrets.token_hex(32));changed=True
if not changed:raise SystemExit('Compose is already initialized; existing credentials are unchanged.')
fd,name=tempfile.mkstemp(prefix='.compose-init-',dir=root)
try:
 os.fchmod(fd,0o600)
 with os.fdopen(fd,'w')as f:f.write(text);f.flush();os.fsync(f.fileno())
 os.replace(name,p)
except BaseException:
 Path(name).unlink(missing_ok=True);raise
print('Initialized docker-compose.yml (0600). Credentials were not printed. Keep this private file out of Git.')
