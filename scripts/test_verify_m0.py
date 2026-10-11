import contextlib,importlib.util,io,json,sys,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('verify_m0',Path(__file__).with_name('verify-m0.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class RuntimeConfigurationTests(unittest.TestCase):
 def test_database_profiles_do_not_require_provider_env_secrets(self):
  calls=[]
  def request(url,method='GET',body=None,headers=None,read_limit=65536):
   calls.append((url,method,body,headers))
   if url.endswith('/auth/login'):return 200,{'Set-Cookie':'media_tool_session=PRIVATE-SESSION; HttpOnly; Path=/'},b'{"data":{"authenticated":true}}'
   if url.endswith('/preflight'):data={'cloudConfigured':True,'cloudSource':'database','embyConfigured':True,'embySource':'database','gatewayEnabled':True,'legacyKeyEnabled':False}
   elif url.endswith('/strm'):return 200,{},b'https://owned/api/v1/cloudStorage/strm302?assetId=fixture&apiKey=PRIVATE-GRANT&path=%2F115%2Fold.mkv\n'
   elif url.endswith('/validation-ref'):data={'path':'/115/current.mkv'}
   elif url.endswith('/cloudStorage/test'):self.assertEqual(body['path'],'/115/current.mkv');data={'pathReadable':True}
   elif url.endswith('/diagnose'):data={'playback':{'targetValidated':True}}
   elif url.endswith('/emby/test'):data={}
   elif url.endswith('/emby/check'):data={'canPlay':True,'sourceMapped':True}
   else:return 302,{'X-Media-Gateway':'emby-static'},b''
   return 200,{},json.dumps({'data':data}).encode()
  values={'ADMIN_USERNAME':'fixture_admin','M0_EMBY_USER_TOKEN':'PRIVATE-USER','M0_EMBY_ITEM_ID':'item'}
  with tempfile.TemporaryDirectory() as d,patch.object(m,'getpass') as gp,patch.object(m.sys.stdin,'isatty',return_value=True),patch.object(m,'env_values',return_value=values),patch.object(m,'request',side_effect=request),patch.object(m,'read_media',return_value=(True,{'status':206})),patch.object(sys,'argv',['verify-m0.py','--asset-id','fixture','--gateway-url','http://localhost:18091','--report',str(Path(d)/'report.json')]):
   gp.getpass.return_value='PRIVATE-ADMIN-PASSWORD'
   out=io.StringIO()
   with contextlib.redirect_stdout(out):result=m.main()
   self.assertEqual(result,0);text=out.getvalue();self.assertNotIn('PRIVATE-',text);report=json.loads((Path(d)/'report.json').read_text());self.assertTrue(report['serverChainVerified']);self.assertFalse(report['realPlayerVerified']);self.assertEqual((Path(d)/'report.json').stat().st_mode&0o777,0o600)
   self.assertTrue(any(url.startswith('http://localhost:18091/Videos/') for url,_,_,_ in calls))
 def test_missing_profiles_report_configuration_without_playback(self):
  with tempfile.TemporaryDirectory() as d,patch.object(m,'env_values',return_value={}),patch.object(m,'request',return_value=(200,{},b'{"data":{"cloudConfigured":false}}')),patch.object(sys,'argv',['verify','--report',str(Path(d)/'report.json')]),contextlib.redirect_stdout(io.StringIO()):
   self.assertEqual(m.main(),2);report=json.loads((Path(d)/'report.json').read_text());self.assertEqual(report['status'],'needs_configuration');self.assertFalse(report['serverChainVerified'])
if __name__=='__main__':unittest.main()
