#!/usr/bin/env python3
"""Read-only M0 checks. Never print keys/URLs or claim real-player decoding."""
import argparse,getpass,http.cookies,datetime,http.client,ipaddress,json,os,shlex,socket,ssl,sys,tempfile,urllib.error,urllib.parse,urllib.request
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*args,**kwargs):return None
opener=urllib.request.build_opener(NoRedirect())

def env_values():
 values=dict(os.environ)
 env=ROOT/'.env'
 if env.exists():
  for line in env.read_text().splitlines():
   if not line.strip() or line.lstrip().startswith('#') or '=' not in line:continue
   key,raw=line.split('=',1);parts=shlex.split(raw,comments=True,posix=True)
   if len(parts)>1:raise ValueError('quote values containing spaces in .env')
   if key.strip() not in values:values[key.strip()]=parts[0] if parts else ''
 return values

def request(url,method='GET',body=None,headers=None,read_limit=65536):
 data=json.dumps(body).encode() if body is not None else None
 req=urllib.request.Request(url,data=data,method=method,headers=headers or {})
 if body is not None:req.add_header('Content-Type','application/json')
 try:response=opener.open(req,timeout=15)
 except urllib.error.HTTPError as error:response=error
 with response:
  payload=response.read(read_limit) if method!='HEAD' else b''
  return response.status,response.headers,payload

def cdn_request(url,headers,limit):
 parsed=urllib.parse.urlsplit(url)
 if parsed.scheme!='https' or parsed.username or parsed.password or not parsed.hostname or parsed.fragment:raise ValueError('unsafe CDN URL')
 addresses=socket.getaddrinfo(parsed.hostname,parsed.port or 443,type=socket.SOCK_STREAM)
 blocked=[ipaddress.ip_network(n) for n in ('64:ff9b::/96','64:ff9b:1::/48','2002::/16','2001::/32')]
 if not addresses or not all(ipaddress.ip_address(item[4][0]).is_global and not any(ipaddress.ip_address(item[4][0]) in net for net in blocked) for item in addresses):raise ValueError('unsafe CDN DNS')
 class PinnedHTTPS(http.client.HTTPSConnection):
  def connect(self):
   last=None
   for family,socktype,proto,canonname,address in addresses:
    raw=socket.socket(family,socktype,proto);raw.settimeout(self.timeout)
    try:
     raw.connect(address);self.sock=ssl.create_default_context().wrap_socket(raw,server_hostname=parsed.hostname);return
    except Exception as error:raw.close();last=error
   raise OSError('CDN connection failed') from last
 connection=PinnedHTTPS(parsed.hostname,parsed.port or 443,timeout=15)
 try:
  target=urllib.parse.urlunsplit(('', '', parsed.path or '/', parsed.query, ''))
  connection.request('GET',target,headers=headers)
  response=connection.getresponse()
  return response.status,response.headers,response.read(limit)
 finally:connection.close()

def read_media(url,headers,limit=1024,backend=False):
 for attempt in range(5):
  if backend:status,h,body=request(url,headers=headers,read_limit=limit)
  else:status,h,body=cdn_request(url,headers,limit)
  if status in (301,302,303,307,308):
   location=h.get('Location','');url=urllib.parse.urljoin(url,location)
   # Provider credentials are never copied from the gateway request to a CDN.
   headers={k:v for k,v in headers.items() if k in ('User-Agent','Range')};backend=False
   continue
  media_type=h.get('Content-Type','').split(';')[0]
  ok=status in (200,206) and len(body)>0 and (media_type.startswith(('video/','audio/')) or media_type=='application/octet-stream')
  return ok,{'status':status,'sampleBytes':len(body),'delivery':h.get('X-Media-Delivery','cdn'),'rangeResponse':status==206}
 raise ValueError('redirect limit exceeded')

def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--base-url',default='http://localhost:18080')
 parser.add_argument('--username',help='Registered administrator username; otherwise ADMIN_USERNAME')
 parser.add_argument('--password-file',help='Private 0600 file containing administrator password; otherwise prompt interactively')
 parser.add_argument('--gateway-url',help='Dedicated Emby gateway origin; defaults to the management /emby mount')
 parser.add_argument('--asset-id',help='Existing indexed asset UUID; obtains its STRM through authenticated API')
 parser.add_argument('--item-id',help='Emby item ID; defaults to M0_EMBY_ITEM_ID')
 parser.add_argument('--path',help='CloudDrive2 test media path; defaults to M0_MEDIA_PATH')
 parser.add_argument('--user-agent',default='MediaTool-M0/0.2')
 parser.add_argument('--report',default='data/diagnostics/m0-report.json')
 args=parser.parse_args()
 admin_headers=None;base=None
 report={'checkedAt':datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat(),'checks':[],'serverChainVerified':False,'realPlayerVerified':False}
 def record(name,passed,detail):report['checks'].append({'name':name,'passed':passed,'detail':detail})
 try:
  values=env_values();path=args.path or values.get('M0_MEDIA_PATH','')
  parsed=urllib.parse.urlsplit(args.base_url)
  if parsed.scheme not in ('http','https') or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment or parsed.path not in ('','/'):
   raise ValueError('base URL must be an HTTP(S) origin')
  if parsed.scheme=='http' and parsed.hostname not in ('localhost','127.0.0.1','::1'):raise ValueError('use HTTPS when sending admin credentials outside loopback')
  base=args.base_url.rstrip('/')
  username=args.username or values.get('ADMIN_USERNAME','')
  password=None
  password_file=args.password_file or values.get('ADMIN_PASSWORD_FILE','')
  if password_file:
   private=Path(password_file)
   if not private.is_file() or private.stat().st_mode&0o077:raise ValueError('password file must be private')
   password=private.read_text().removesuffix('\n').removesuffix('\r')
  elif username and sys.stdin.isatty():password=getpass.getpass('管理员密码: ')
  missing=[];preflight={};status=0
  if not username or password is None:missing.append('registered administrator username and interactive password or private password file')
  else:
   login_headers={'X-Requested-With':'media-tool','User-Agent':args.user_agent}
   login_status,login_reply,_=request(base+'/api/v1/auth/login','POST',{'username':username,'password':password},login_headers)
   password=None
   cookies=http.cookies.SimpleCookie()
   for raw_cookie in (login_reply.get_all('Set-Cookie',[]) if hasattr(login_reply,'get_all') else [login_reply.get('Set-Cookie','')]):cookies.load(raw_cookie)
   if login_status!=200 or 'media_tool_session' not in cookies:missing.append('valid administrator account login')
   else:
    admin_headers={'Cookie':'media_tool_session='+cookies['media_tool_session'].value,'X-Requested-With':'media-tool','User-Agent':args.user_agent}
    status,_,raw=request(base+'/api/v1/diagnostics/playback/preflight',headers=admin_headers)
    preflight=json.loads(raw).get('data',{}) if status==200 else {}
    if status!=200:missing.append('authenticated runtime preflight')
  for field in ('cloudConfigured','embyConfigured','gatewayEnabled'):
   if not preflight.get(field):missing.append(field)
  asset=args.asset_id or values.get('M0_ASSET_ID','')
  item=args.item_id or values.get('M0_EMBY_ITEM_ID','')
  if not item:missing.append('M0_EMBY_ITEM_ID or --item-id')
  if not values.get('M0_EMBY_USER_TOKEN'):missing.append('M0_EMBY_USER_TOKEN')
  if not asset and not path:missing.append('M0_ASSET_ID/--asset-id or M0_MEDIA_PATH/--path')
  if not asset and (not preflight.get('legacyKeyEnabled') or not values.get('STRM_API_KEY')):missing.append('asset ID (recommended; legacy playback unavailable)')
  gateway=base+'/emby'
  if args.gateway_url:
   gp=urllib.parse.urlsplit(args.gateway_url)
   if gp.scheme not in ('http','https') or not gp.hostname or gp.username or gp.password or gp.query or gp.fragment or gp.path not in ('','/'):
    raise ValueError('gateway URL must be an HTTP(S) origin')
   if gp.scheme=='http' and gp.hostname not in ('localhost','127.0.0.1','::1'):raise ValueError('HTTPS required for non-loopback gateway')
   gateway=args.gateway_url.rstrip('/')
  if missing:
   report['status']='needs_configuration';report['missing']=missing
   record('configuration',False,'Configure services in the local management UI; provider secrets are not needed by this script.')
  else:
   def api(endpoint,body):
    status,headers,payload=request(base+'/api/v1'+endpoint,'POST',body,admin_headers)
    try:decoded=json.loads(payload)
    except (ValueError,UnicodeDecodeError):return False,{}
    return status==200,decoded.get('data',{})
   stable=None
   if asset:
    status,_,raw=request(base+'/api/v1/media/assets/'+urllib.parse.quote(asset,safe='')+'/strm',headers=admin_headers,read_limit=8193)
    if status!=200 or len(raw)>8192:raise ValueError('managed STRM unavailable')
    supplied=urllib.parse.urlsplit(raw.decode().strip())
    if supplied.scheme not in ('http','https') or supplied.path!='/api/v1/cloudStorage/strm302' or supplied.username or supplied.password or supplied.fragment:raise ValueError('invalid STRM')
    q=urllib.parse.parse_qs(supplied.query,strict_parsing=True)
    if q.get('assetId')!=[asset] or any(len(v)!=1 for v in q.values()):raise ValueError('invalid asset grant')
    status,_,raw=request(base+'/api/v1/media/assets/'+urllib.parse.quote(asset,safe='')+'/validation-ref',headers=admin_headers)
    if status!=200:raise ValueError('indexed media path unavailable')
    path=json.loads(raw).get('data',{}).get('path','')
    if not path:raise ValueError('indexed path missing')
    stable=base+supplied.path+'?'+supplied.query
   connected,probe=api('/cloudStorage/test',{'path':path});record('cloud_read_access',connected and probe.get('pathReadable',False),'Read-only gRPC authentication and file lookup.')
   diagnosed,diagnosis=api('/cloudStorage/diagnose',{'path':path});record('cloud_link_resolution',diagnosed and diagnosis.get('playback',{}).get('targetValidated',False),'Current cloud URL obtained and target policy checked; raw URL omitted.')
   emby_ready,_=api('/emby/test',{});record('emby_reachable',emby_ready,'Public Emby information endpoint.')
   authorized,mapping=api('/emby/check',{'itemId':item,'mediaSourceId':values.get('M0_EMBY_MEDIA_SOURCE_ID',''),'userToken':values['M0_EMBY_USER_TOKEN']})
   record('emby_permission_mapping',authorized and mapping.get('canPlay',False) and mapping.get('sourceMapped',False),'Caller permissions and managed STRM mapping; no privileged service token.')
   if connected and diagnosed:
    if stable is None:stable=base+'/api/v1/cloudStorage/strm302?'+urllib.parse.urlencode({'apiKey':values['STRM_API_KEY'],'path':path})
    headers={'User-Agent':args.user_agent,'Range':'bytes=0-1023'}
    status,h,_=request(stable,'HEAD',headers=headers)
    record('strm_head',status in (200,206,302),{'status':status,'delivery':h.get('X-Media-Delivery','unknown')})
    ok,detail=read_media(stable,headers,backend=True);record('strm_media_bytes',ok,detail)
    stream=gateway+'/Videos/'+urllib.parse.quote(item,safe='')+'/stream?'+urllib.parse.urlencode({'Static':'true','MediaSourceId':values.get('M0_EMBY_MEDIA_SOURCE_ID','')})
    headers={'X-Emby-Token':values['M0_EMBY_USER_TOKEN'],'User-Agent':args.user_agent,'Range':'bytes=0-1023'}
    status,h,_=request(stream,'HEAD',headers=headers)
    mapped=h.get('X-Media-Gateway')=='emby-static'
    record('emby_static_gateway',status in (200,206,302) and mapped,{'status':status,'intercepted':mapped})
    ok,detail=read_media(stream,headers,backend=True);record('emby_media_bytes',ok,detail)
   report['serverChainVerified']=bool(report['checks']) and all(check['passed'] for check in report['checks'])
   report['status']='server_chain_verified' if report['serverChainVerified'] else 'verification_failed'
 except Exception:
  report['status']='verification_failed';record('execution',False,'Request or configuration failed. Check local service and settings; raw exceptions omitted to protect URLs/keys.')
 if base and admin_headers:
  try:request(base+'/api/v1/auth/logout','POST',{},admin_headers)
  except Exception:pass  # expires on server; raw exceptions must not expose cookies
 target=Path(args.report);target=target if target.is_absolute() else ROOT/target
 target.parent.mkdir(parents=True,exist_ok=True)
 fd,tmp=tempfile.mkstemp(prefix='.m0-',dir=target.parent)
 try:
  with os.fdopen(fd,'w') as file:
   json.dump(report,file,ensure_ascii=False,indent=2);file.write('\n');file.flush();os.fsync(file.fileno())
  os.replace(tmp,target)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)
 print(json.dumps(report,ensure_ascii=False,indent=2))
 print('Real player verification remains manual: start, seek, resume, audio/subtitles, and transcode.')
 return 0 if report['serverChainVerified'] else 2
if __name__=='__main__':sys.exit(main())
