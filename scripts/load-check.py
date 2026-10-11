#!/usr/bin/env python3
"""Bounded unauthenticated readiness load check; not a media throughput test."""
import argparse,concurrent.futures,datetime,json,statistics,time,urllib.parse,urllib.request
from pathlib import Path
import tempfile,os
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*args,**kwargs):return None
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--base-url',default='http://localhost:18080');p.add_argument('--requests',type=int,default=500);p.add_argument('--concurrency',type=int,default=16);p.add_argument('--report',default='data/diagnostics/readiness-load.json');a=p.parse_args()
 u=urllib.parse.urlsplit(a.base_url)
 if u.scheme not in ('http','https')or not u.hostname or u.username or u.password or u.query or u.fragment or u.path not in ('','/'):p.error('HTTP(S) origin required')
 if not 1<=a.requests<=10000 or not 1<=a.concurrency<=64:p.error('bounded requests/concurrency required')
 def one(_):
  start=time.monotonic()
  try:
   with urllib.request.build_opener(NoRedirect()).open(a.base_url.rstrip('/')+'/readyz',timeout=10)as r:ok=r.status==200 and json.loads(r.read(4096)).get('data',{}).get('status')=='ready'
  except Exception:ok=False
  return ok,(time.monotonic()-start)*1000
 start=time.monotonic()
 with concurrent.futures.ThreadPoolExecutor(max_workers=a.concurrency)as e:results=list(e.map(one,range(a.requests)))
 elapsed=time.monotonic()-start;latencies=sorted(v[1]for v in results)
 report={'checkedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'scope':'readiness API only; no media/CDN/client throughput claim','requests':a.requests,'concurrency':a.concurrency,'succeeded':sum(v[0]for v in results),'failed':sum(not v[0]for v in results),'elapsedSeconds':round(elapsed,3),'requestsPerSecond':round(a.requests/elapsed,1),'latencyMs':{'median':round(statistics.median(latencies),2),'p95':round(latencies[int((len(latencies)-1)*.95)],2),'max':round(max(latencies),2)}}
 target=Path(a.report);target.parent.mkdir(parents=True,exist_ok=True);fd,tmp=tempfile.mkstemp(prefix='.load-',dir=target.parent)
 try:
  with os.fdopen(fd,'w')as f:json.dump(report,f,ensure_ascii=False,indent=2)
  os.replace(tmp,target)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)
 print(json.dumps(report,ensure_ascii=False,indent=2));return 0 if report['failed']==0 else 1
if __name__=='__main__':raise SystemExit(main())
