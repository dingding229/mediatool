#!/usr/bin/env python3
"""Run both locally-built image architectures without accounts or volumes."""
import datetime,json,os,subprocess,tempfile,time,urllib.request,uuid
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
results=[]
for architecture,image in [('arm64','media-tool:local'),('amd64','media-tool:verify-amd64')]:
 metadata=json.loads(subprocess.check_output(['docker','image','inspect',image],text=True))[0]
 if metadata['Architecture']!=architecture:raise RuntimeError('wrong image architecture')
 name='media-tool-smoke-'+uuid.uuid4().hex[:12];started=False
 try:
  subprocess.run(['docker','run','-d','--rm','--platform','linux/'+architecture,'--name',name,'-p','127.0.0.1::8080','-e','APP_ADDR=0.0.0.0:8080','-e','EMBY_GATEWAY_ADDR=0.0.0.0:8091','-e','SESSION_SECRET_KEY='+'a'*64,'-e','STRM_API_KEY='+'b'*64,'-e','APP_PUBLIC_URL=http://localhost:8080',image],check=True,stdout=subprocess.DEVNULL);started=True
  container=json.loads(subprocess.check_output(['docker','inspect',name],text=True))[0];binding=container['NetworkSettings']['Ports']['8080/tcp'][0]
  if binding['HostIp']!='127.0.0.1':raise RuntimeError('smoke port was not private')
  url='http://127.0.0.1:'+binding['HostPort']+'/healthz'
  for attempt in range(40):
   try:
    with urllib.request.urlopen(url,timeout=2)as r:assert r.status==200 and json.loads(r.read(4096))['data']['status']=='alive'
    break
   except Exception:
    if attempt==39:raise RuntimeError('container liveness failed')
    time.sleep(.5)
  subprocess.run(['docker','run','--rm','--platform','linux/'+architecture,'--entrypoint','/app/rekey',image,'--help'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
  results.append({'architecture':'linux/'+architecture,'imageId':metadata['Id'],'serverHealthz':True,'rekeyBinaryRunnable':True,'providerAcceptance':False,'scope':'isolated startup; no DB/accounts/volumes; amd64 emulated on arm64 host'})
 finally:
  if started:subprocess.run(['docker','stop',name],check=True,stdout=subprocess.DEVNULL)
report={'checkedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'results':results}
target=ROOT/'data/diagnostics/image-smoke.json';target.parent.mkdir(parents=True,exist_ok=True);fd,tmp=tempfile.mkstemp(prefix='.smoke-',dir=target.parent)
try:
 with os.fdopen(fd,'w')as f:json.dump(report,f,indent=2)
 os.replace(tmp,target)
finally:
 if os.path.exists(tmp):os.unlink(tmp)
print(json.dumps(report,indent=2))
