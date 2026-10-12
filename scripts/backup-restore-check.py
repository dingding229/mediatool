#!/usr/bin/env python3
"""Private snapshot backup + clone-only migration/ciphertext/immutability verification."""
import argparse,gzip,hashlib,json,os,re,secrets,subprocess,time
from pathlib import Path

def call(args,**kwargs):
    result=subprocess.run(args,stderr=subprocess.PIPE,**kwargs)
    if result.returncode:raise RuntimeError('Backup/clone command failed; raw diagnostics and credentials omitted')
    return result

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root',type=Path,default=Path(__file__).resolve().parent.parent)
    parser.add_argument('--archive',type=Path,help='existing private .dump or .sql.gz; restore only into a fresh owned clone')
    args=parser.parse_args();root=args.root.resolve();compose=['docker','compose','--project-directory',str(root),'-f',str(root/'docker-compose.yml')]
    services=call(compose+['config','--services'],stdout=subprocess.PIPE,text=True).stdout.split()
    if set(services)!={'app','postgres','redis'}:raise RuntimeError('Only the official three-service Compose is supported')
    name='media_tool_restore_'+secrets.token_hex(6);created=False;exporter=None;source=None
    backup=args.archive
    try:
        if backup:
            backup=backup.resolve()
            if not backup.is_file()or backup.is_symlink()or backup.stat().st_mode&0o077:raise RuntimeError('Existing archive must be a private regular file')
        else:
            directory=root/'data/backups';directory.mkdir(parents=True,exist_ok=True);directory.chmod(0o700)
            backup=directory/('media-tool-'+time.strftime('%Y%m%d-%H%M%S')+'-'+secrets.token_hex(3)+'.dump')
            exporter=subprocess.Popen(compose+['exec','-T','app','/app/backup-verify','-source-snapshot'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
            line=exporter.stdout.readline();source=json.loads(line)
            if not re.fullmatch(r'[0-9a-fA-F]+-[0-9a-fA-F]+-\d+',source['snapshot']):raise RuntimeError('Invalid snapshot receipt')
            fd=os.open(backup,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
            with os.fdopen(fd,'wb')as out:
                call(compose+['exec','-T','postgres','sh','-c','pg_dump -Fc --snapshot "$1" -U "$POSTGRES_USER" -d "$POSTGRES_DB"','sh',source['snapshot']],stdout=out)
                out.flush();os.fsync(out.fileno())
            exporter.stdin.write('release\n');exporter.stdin.flush();exporter.communicate(timeout=30)
            if exporter.returncode:raise RuntimeError('Snapshot exporter did not finish')
            exporter=None
        call(compose+['exec','-T','postgres','sh','-c','createdb -U "$POSTGRES_USER" "$1"','sh',name]);created=True
        if backup.name.endswith('.sql.gz'):
            extractor=subprocess.Popen(['gzip','-dc','--',str(backup)],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
            try:
                call(compose+['exec','-T','postgres','sh','-c','psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1"','sh',name],stdin=extractor.stdout,stdout=subprocess.DEVNULL)
                extractor.communicate(timeout=30)
                if extractor.returncode:raise RuntimeError('Archive decompression failed')
            finally:
                if extractor.poll()is None:extractor.kill();extractor.communicate()
        else:
            with backup.open('rb')as stream:
                call(compose+['exec','-T','postgres','sh','-c','pg_restore --exit-on-error --no-owner -U "$POSTGRES_USER" -d "$1"','sh',name],stdin=stream,stdout=subprocess.DEVNULL)
        output=call(compose+['exec','-T','-e','BACKUP_VERIFY_DB='+name,'app','/app/backup-verify','-database',name],stdout=subprocess.PIPE,text=True).stdout
        restored=json.loads(output)
        if source and source['tables']!=restored['initialTables']:raise RuntimeError('Source snapshot and restored table checksums differ')
        sha=hashlib.sha256()
        with backup.open('rb')as stream:
            for chunk in iter(lambda:stream.read(1<<20),b''):sha.update(chunk)
        report={'archive':str(backup),'sha256':sha.hexdigest(),'permissions':'0600','sourceSnapshotMatched':True if source else None,'existingBaselineArchive':bool(args.archive),'restoreDatabaseWasTemporary':True,'credentialsMasterKeyIncluded':False,'restored':restored}
    finally:
        if exporter:
            try:exporter.communicate('release\n',timeout=5)
            except subprocess.TimeoutExpired:exporter.kill();exporter.communicate()
        if created:
            call(compose+['exec','-T','postgres','sh','-c','dropdb -U "$POSTGRES_USER" "$1"','sh',name])
    report['temporaryDatabaseDeleted']=True
    target=root/'data/diagnostics'/('backup-restore-'+time.strftime('%Y%m%d-%H%M%S')+'-'+secrets.token_hex(3)+'.json');target.parent.mkdir(parents=True,exist_ok=True)
    fd=os.open(target,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
    with os.fdopen(fd,'w')as out:json.dump(report,out,ensure_ascii=False,indent=2)
    print(json.dumps(report,ensure_ascii=False,indent=2))

if __name__=='__main__':main()
