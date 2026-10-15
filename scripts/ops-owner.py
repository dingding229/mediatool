#!/usr/bin/env python3
"""Owner-only fixed maintenance runner. No socket/API/daemon is installed in app.

Web approves a short-lived request; this local operator command validates and
executes it under an exclusive host lock. An uncertain execution is never replayed.
"""
import argparse,fcntl,hashlib,json,os,re,shutil,subprocess,tempfile,time,urllib.parse
from pathlib import Path

SHA=re.compile(r'^[a-f0-9]{40}$')
UUID=re.compile(r'^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$')
IMAGE=re.compile(r'^(?:docker.io/)?dingding229/mediatool:sha-([a-f0-9]{40})$')

def checked_image(image,info):
    match=IMAGE.fullmatch(image)
    by_digest=re.fullmatch(r'(?:docker.io/)?dingding229/mediatool@(sha256:[a-f0-9]{64})',image)
    if not(match or by_digest)or info.get('Os')!='linux' or info.get('Architecture')!='amd64':raise RuntimeError('Only immutable Docker Hub AMD64 images are accepted')
    labels=info.get('Config',{}).get('Labels')or{}
    revision=labels.get('org.opencontainers.image.revision','')
    if not SHA.fullmatch(revision)or(match and revision!=match[1]):raise RuntimeError('Image revision mismatch')
    schema=labels.get('org.opencontainers.image.schema','')
    if not schema.isdecimal():raise RuntimeError('Target image has no verified schema label; rollback is blocked')
    digests=[d.split('@',1)[1]for d in info.get('RepoDigests',[])if re.fullmatch(r'(?:docker.io/)?dingding229/mediatool@sha256:[a-f0-9]{64}',d)]
    if not digests or(by_digest and by_digest[1]not in digests):raise RuntimeError('Docker Hub digest unavailable or changed')
    return revision,int(schema),by_digest[1]if by_digest else digests[0]

def replace_image(text,image):
    if not(IMAGE.fullmatch(image)or re.fullmatch(r'dingding229/mediatool@sha256:[a-f0-9]{64}',image))or '${'in text:raise RuntimeError('Private Compose must use direct values')
    result,count=re.subn(r'(?m)^(    image:\s*)[^\n]+',lambda m:m[1]+image,text,count=1)
    if count!=1:raise RuntimeError('Expected exactly one app image line')
    return result

def restore_dsn(dsn,candidate):
    if not re.fullmatch(r'media_tool_restore_[a-f0-9]{12}',candidate):raise RuntimeError('Invalid verified candidate database')
    u=urllib.parse.urlsplit(dsn)
    if urllib.parse.unquote(u.path)=='/'+candidate:raise RuntimeError('Candidate must be a different database')
    return urllib.parse.urlunsplit(u._replace(path='/'+candidate))

def atomic(path,content):
    fd,name=tempfile.mkstemp(prefix='.ops-',dir=path.parent)
    try:
        os.fchmod(fd,0o600)
        with os.fdopen(fd,'w')as f:f.write(content);f.flush();os.fsync(f.fileno())
        os.replace(name,path)
    finally:Path(name).unlink(missing_ok=True)

class Owner:
    def __init__(self,root):
        self.root=root.resolve();self.compose=self.root/'docker-compose.yml'
        if not self.compose.is_file()or self.compose.is_symlink():raise RuntimeError('Private root Compose is required')
        self.base=['docker','compose','-f',str(self.compose)]
        services=self.call(self.base+['config','--services']).split()
        if sorted(services)!=['app','postgres','redis']:raise RuntimeError('Only the canonical three services are allowed')
        cid=self.call(self.base+['ps','-q','app']).strip()
        if not cid:raise RuntimeError('Current app container is required')
        self.app=json.loads(self.call(['docker','inspect',cid]))[0]
        self.env=dict(v.split('=',1)for v in self.app['Config']['Env']if '='in v)
        self.image=self.app['Config']['Image'];self.control=self.base+['exec','-T','app','/app/ops-control']
        pgid=self.call(self.base+['ps','-q','postgres']).strip()
        pg=json.loads(self.call(['docker','inspect',pgid]))[0]
        shared=set(pg['NetworkSettings']['Networks'])&set(self.app['NetworkSettings']['Networks'])
        aliases={pg['Name'].lstrip('/')}
        for network in shared:
            metadata=pg['NetworkSettings']['Networks'][network]
            aliases.update(metadata.get('Aliases')or[]);aliases.add(metadata.get('IPAddress',''))
        dsn=urllib.parse.urlsplit(self.env['DATABASE_URL'])
        if not shared or dsn.hostname not in aliases or(dsn.port or 5432)!=5432:raise RuntimeError('App database must be the canonical same-project PostgreSQL container')

    def call(self,args,timeout=2400,stdin=None):
        p=subprocess.run(args,cwd=self.root,input=stdin,text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=timeout)
        if p.returncode:raise RuntimeError(('Fixed maintenance command failed: '+args[0]+' '+(args[1]if len(args)>1 else'')+'; '+p.stderr[-4000:]).strip())
        return p.stdout
    def cli(self,*args):return json.loads(self.call(self.control+list(args)))
    def inspect_image(self,image):
        self.call(['docker','pull','--platform','linux/amd64',image])
        info=json.loads(self.call(['docker','image','inspect',image]))[0]
        return checked_image(image,info)
    def register(self,image):
        rev,schema,digest=self.inspect_image(image)
        self.cli('register-release','-revision',rev,'-schema',str(schema),'-digest',digest)
        return {'registered':True,'revision':rev,'schema':schema}
    def sql(self,query,database=None):
        return self.call(self.base+['exec','-T','postgres','sh','-c','exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -At -c "$2"','sh',database or urllib.parse.unquote(urllib.parse.urlsplit(self.env['DATABASE_URL']).path.lstrip('/')),query]).strip()
    def execute(self,id):
        if not UUID.fullmatch(id):raise RuntimeError('Invalid maintenance request')
        request=self.cli('check','-id',id)
        if request['state']!='approved':raise RuntimeError('Only an approved, unexpired request may run')
        current_info=self.cli('info')
        current=checked_image(self.image,json.loads(self.call(['docker','image','inspect',self.app['Image']]))[0])
        rev,schema,digest=current
        if request['kind']=='rollback':
            expected=request['payload'].get('imageDigest','')
            if not re.fullmatch(r'sha256:[a-f0-9]{64}',expected):raise RuntimeError('Approved image digest is missing')
            rev,schema,digest=self.inspect_image('dingding229/mediatool@'+expected)
            if rev!=request['payload']['targetRevision']or digest!=expected:raise RuntimeError('Target differs from approved version')
        if schema!=current_info['schema']or schema!=request['payload']['schema']:raise RuntimeError('Schema differs; no automatic downgrade')
        target='dingding229/mediatool@'+digest
        self.cli('register-release','-revision',current[0],'-schema',str(current[1]),'-digest',current[2])
        directory=self.root/'private'/('ops-execution-'+id)
        directory.mkdir(mode=0o700,parents=True,exist_ok=False)
        old=self.compose.read_text();atomic(directory/'docker-compose.before.yml',old)
        binary=None;hostenv=None
        if request['kind']=='restore':
            binary=directory/'ops-control';self.call(['docker','cp',self.app['Id']+':/app/ops-control',str(binary)]);binary.chmod(0o700)
            hostenv=self.host_environment(directory)
        size=int(self.sql('SELECT pg_database_size(current_database())'))
        free_line=self.call(self.base+['exec','-T','postgres','sh','-c','df -Pk "$PGDATA"']).splitlines()[-1].split()
        if len(free_line)<4 or int(free_line[3])*1024<size*2+512*1024*1024:raise RuntimeError('PostgreSQL storage headroom is insufficient for safe maintenance')

        # Capture both the DB and owned outputs before locking the application.
        backup=self.cli('backup');atomic(directory/'preflight-backup.json',json.dumps(backup))
        self.cli('claim','-id',id)
        atomic(directory/'request.json',json.dumps(request));atomic(directory/'status.json',json.dumps({'state':'claimed','id':id}))
        stopped=False;switched=False
        try:
            # Existing business calls must drain; worker claim and management writes
            # are fenced by ops_mode. No lease stealing, cancellation or task replay.
            deadline=time.monotonic()+120
            while int(self.sql("SELECT count(*)FROM jobs WHERE state='running' AND kind NOT LIKE 'ops.%'")):
                if time.monotonic()>deadline:raise RuntimeError('Business tasks did not drain; operation blocked')
                time.sleep(2)
            self.call(self.base+['stop','-t','120','app']);stopped=True
            actual=int(self.sql("SELECT coalesce(max(version_id),0) FROM(SELECT DISTINCT ON(version_id)version_id,is_applied FROM goose_db_version ORDER BY version_id,id DESC)v WHERE is_applied"))
            if actual!=schema:raise RuntimeError('Live schema changed after approval')
            for table,expected in request['payload']['protected'].items():
                if not re.fullmatch(r'[a-z][a-z0-9_]{0,62}',table)or not re.fullmatch(r'[a-f0-9]{32}',expected):raise RuntimeError('Invalid protection proof')
                expression="(to_jsonb(t)-'last_used_at')"if table=='external_api_keys'else'to_jsonb(t)'
                actual=self.sql("SELECT coalesce(md5(string_agg("+expression+"::text,E'\\n' ORDER BY "+expression+"::text)),md5(''))FROM "+table+" t")
                if actual!=expected:raise RuntimeError('Protected data changed while draining: '+table)

            if request['kind']=='restore':
                # Static CLI copied from the verified running image; no extra service
                # or container. Host has no need to install a second app runtime.
                prepared=self.host_cli(binary,hostenv,'prepare','-id',id)
                candidate=prepared['candidateDatabase'];restore_dsn(self.env['DATABASE_URL'],candidate)
                oldname=urllib.parse.unquote(urllib.parse.urlsplit(self.env['DATABASE_URL']).path.lstrip('/'))
                olddb='media_tool_before_'+id.replace('-','')[:12]
                if oldname in('postgres','template0','template1')or not oldname:raise RuntimeError('System database cannot be restored')
                quote=lambda value:'"'+value.replace('"','""')+'"'
                literal=lambda value:"'"+value.replace("'","''")+"'"
                atomic(directory/'candidate.json',json.dumps({'candidateDatabase':candidate,'oldDatabaseRetained':olddb,'liveDatabase':oldname}))
                # One transaction swaps names; the original database and every external
                # volume remain intact. No Compose credentials or DB URL are changed.
                self.sql("SELECT pg_terminate_backend(pid)FROM pg_stat_activity WHERE backend_type='client backend'AND datname IN("+literal(oldname)+","+literal(candidate)+")AND pid<>pg_backend_pid()",database='postgres')
                switched=True
                self.sql('BEGIN;ALTER DATABASE '+quote(oldname)+' RENAME TO '+quote(olddb)+';ALTER DATABASE '+quote(candidate)+' RENAME TO '+quote(oldname)+';COMMIT;',database='postgres')
                atomic(self.compose,replace_image(old,target))
            else:atomic(self.compose,replace_image(old,target))
            switched=True
            self.call(self.base+['up','-d','--no-deps','--wait','--wait-timeout','180','app'])
            # Use the new app's environment and retain paused mode until admin review.
            self.control=self.base+['exec','-T','app','/app/ops-control']
            self.cli('finish','-id',id,'-success','-message','宿主执行完成；原库和回滚备份保留。业务保持暂停，须核对后显式解除维护。')
            receipt={'state':'completed','id':id,'targetRevision':rev,'schema':schema,'businessPaused':True,'oldDatabasePreserved':True,'rollbackDirectory':str(directory)}
            atomic(directory/'status.json',json.dumps(receipt));self.cleanup_sensitive(directory);return receipt
        except BaseException as error:
            # Do not silently undo a possibly completed database/image switch.
            # Keep exact phase for operator investigation; no auto rerun.
            atomic(directory/'status.json',json.dumps({'state':'uncertain'if switched else'failed','id':id,'appStopped':stopped,'composeSwitched':switched,'automaticRetryAllowed':False,'error':str(error)}))
            if not switched:
                if stopped:self.call(self.base+['start','app'])
                try:self.cli('finish','-id',id,'-message',('宿主执行失败，保持维护暂停：'+str(error))[:1000])
                except Exception:pass
            raise
    def cleanup_sensitive(self,directory):
        # Full Compose and preflight backup remain private for rollback. Fixed tool
        # wrappers contain only container identity, never passwords; remove binaries.
        shutil.rmtree(directory/'bin',ignore_errors=True)
        (directory/'ops-control').unlink(missing_ok=True)
    def host_environment(self,directory):
        env=dict(self.env);dsn=urllib.parse.urlsplit(env['DATABASE_URL'])
        user=self.app['Config'].get('User','')
        match=re.fullmatch(r'([1-9][0-9]*):([0-9]+)',user)
        if not match:raise RuntimeError('Restore requires the official explicit non-root numeric app UID/GID')
        env['OPS_RESTORE_UID']=match[1];env['OPS_RESTORE_GID']=match[2]

        pgid=self.call(self.base+['ps','-q','postgres']).strip();pg=json.loads(self.call(['docker','inspect',pgid]))[0]
        bindings=pg['NetworkSettings']['Ports'].get('5432/tcp')or[]
        if len(bindings)!=1 or bindings[0]['HostIp']not in('127.0.0.1','::1'):raise RuntimeError('Offline control requires a loopback-only PostgreSQL published port')
        auth=dsn.netloc.rsplit('@',1)[0];host='[::1]'if bindings[0]['HostIp']=='::1'else'127.0.0.1';env['DATABASE_URL']=urllib.parse.urlunsplit(dsn._replace(netloc=auth+'@'+host+':'+bindings[0]['HostPort']))
        for key,default in [('STRM_OUTPUT_DIR','/data/strm'),('OPS_DATA_DIR','/data/maintenance')]:
            container=env.get(key,default);matches=[m for m in self.app['Mounts']if container==m['Destination']or container.startswith(m['Destination'].rstrip('/')+'/')]
            if not matches:raise RuntimeError('Owned data mount unavailable')
            mount=max(matches,key=lambda m:len(m['Destination']));env[key]=str(Path(mount['Source'])/container.removeprefix(mount['Destination']).lstrip('/'))
        # Dedicated fixed pg wrappers run only installed PG tools inside the existing
        # postgres container. Arguments remain argv, never shell code or eval.
        tools=directory/'bin';tools.mkdir(mode=0o700);docker=shutil.which('docker')
        for tool in ['pg_dump','pg_restore']:
            script='''#!/usr/bin/python3
import os,subprocess,sys
args=['''+repr(docker)+''','exec','-i','-e','PGHOST=127.0.0.1','-e','PGPORT=5432']
for name in ['PGDATABASE','PGUSER','PGPASSWORD','PGSSLMODE','PGSSLROOTCERT','PGSSLCERT','PGSSLKEY']:
 if name in os.environ:args+=['-e',name]
raise SystemExit(subprocess.call(args+['''+repr(pgid)+','+repr(tool)+''',*sys.argv[1:]]))
'''
            atomic(tools/tool,script);(tools/tool).chmod(0o700)
        env['PATH']=str(tools)+':/usr/bin:/bin';return env
    def host_cli(self,binary,env,*args):
        p=subprocess.run([str(binary),*args],cwd=self.root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=2400)
        if p.returncode:atomic(binary.parent/'offline-error.log',p.stderr);raise RuntimeError('Offline candidate preparation failed; private receipt retained')
        return json.loads(p.stdout)

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--root',type=Path,required=True);sub=parser.add_subparsers(dest='command',required=True)
    register=sub.add_parser('register');register.add_argument('image');execute=sub.add_parser('execute');execute.add_argument('id')
    args=parser.parse_args();root=args.root.resolve()
    if not root.is_dir():raise SystemExit('Existing deployment root required')
    with(root/'.update.lock').open('a')as lock:
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB);owner=Owner(root)
        result=owner.register(args.image)if args.command=='register'else owner.execute(args.id)
        print(json.dumps(result,ensure_ascii=False))
if __name__=='__main__':main()
