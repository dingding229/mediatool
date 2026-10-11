#!/usr/bin/env python3
"""Create a private DB archive and validate it in a new, disposable database."""
import json,os,secrets,subprocess,time
from pathlib import Path
root=Path(__file__).resolve().parent.parent
compose=['docker','compose','--env-file',str(root/'.env'),'-f',str(root/'docker-compose.yml')]
backup=root/'data/backups'/('media-tool-'+time.strftime('%Y%m%d-%H%M%S')+'.dump')
backup.parent.mkdir(parents=True,exist_ok=True)
fd=os.open(backup,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
try:
 with os.fdopen(fd,'wb') as out:
  subprocess.run(compose+['exec','-T','postgres','sh','-c','pg_dump -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],stdout=out,check=True)
  out.flush();os.fsync(out.fileno())
except Exception:
 backup.unlink(missing_ok=True)  # only this invocation's newly-created partial archive
 raise
name='media_tool_restore_'+secrets.token_hex(6)
created=False
try:
 subprocess.run(compose+['exec','-T','postgres','sh','-c','createdb -U "$POSTGRES_USER" "$1"','sh',name],check=True)
 created=True
 with backup.open('rb') as source:
  subprocess.run(compose+['exec','-T','postgres','sh','-c','pg_restore --exit-on-error --no-owner -U "$POSTGRES_USER" -d "$1"','sh',name],stdin=source,check=True)
 query="SELECT json_build_object('migrations', (SELECT max(version_id) FROM goose_db_version WHERE is_applied), 'jobs',(SELECT count(*) FROM jobs), 'integrations',(SELECT count(*) FROM integrations), 'media',(SELECT count(*) FROM media_items), 'publicTables',(SELECT count(*) FROM pg_tables WHERE schemaname='public'));"
 output=subprocess.check_output(compose+['exec','-T','postgres','sh','-c','psql -At -U "$POSTGRES_USER" -d "$1" -c "$2"','sh',name,query],text=True)
 result=json.loads(output.strip())
 expected=max(int(p.name.split('_')[0]) for p in (root/'migrations').glob('*.sql'))
 if result['migrations']!=expected or result['publicTables']<45:raise RuntimeError('restored schema did not meet current migration baseline')
 report={'archive':str(backup),'permissions':'0600','restoreDatabaseWasTemporary':True,'restored':result,'credentialsMasterKeyIncluded':False}
 print(json.dumps(report,ensure_ascii=False,indent=2))
 target=root/'data/diagnostics/backup-restore-report.json';target.parent.mkdir(parents=True,exist_ok=True)
 fd=os.open(target,os.O_CREAT|os.O_TRUNC|os.O_WRONLY,0o600)
 with os.fdopen(fd,'w') as file:json.dump(report,file,ensure_ascii=False,indent=2)
finally:
 if created:subprocess.run(compose+['exec','-T','postgres','sh','-c','dropdb -U "$POSTGRES_USER" "$1"','sh',name],check=True)
