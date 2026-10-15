import importlib.util,unittest
from pathlib import Path
spec=importlib.util.spec_from_file_location('ops_owner',Path(__file__).with_name('ops-owner.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class OwnerGuards(unittest.TestCase):
 def test_image_is_pinned_hub_arch_schema_and_revision(self):
  sha='a'*40;image='dingding229/mediatool:sha-'+sha;info={'Os':'linux','Architecture':'amd64','Config':{'Labels':{'org.opencontainers.image.revision':sha,'org.opencontainers.image.schema':'50'}},'RepoDigests':['dingding229/mediatool@sha256:'+'b'*64]}
  self.assertEqual(m.checked_image(image,info)[1],50)
  self.assertEqual(m.checked_image('dingding229/mediatool@sha256:'+'b'*64,info)[0],sha)
  with self.assertRaises(RuntimeError):m.checked_image('dingding229/mediatool@sha256:'+'c'*64,info)
  for bad in ['dingding229/mediatool:latest','example.com/mediatool:sha-'+sha]:
   with self.assertRaises(RuntimeError):m.checked_image(bad,info)
  info['Config']['Labels'].pop('org.opencontainers.image.schema')
  with self.assertRaises(RuntimeError):m.checked_image(image,info)
 def test_compose_change_is_app_only_and_candidate_db_is_owned(self):
  before='services:\n  app:\n    image: old\n  postgres:\n    image: postgres\n'
  after=m.replace_image(before,'dingding229/mediatool:sha-'+'a'*40);self.assertIn('    image: postgres',after)
  self.assertEqual(m.restore_dsn('postgres://u:p@postgres:5432/media_tool?sslmode=disable','media_tool_restore_'+'b'*12),'postgres://u:p@postgres:5432/media_tool_restore_'+'b'*12+'?sslmode=disable')
  for name in ['media_tool','postgres','media_tool_restore_../../x']:
   with self.assertRaises(RuntimeError):m.restore_dsn('postgres://u:p@postgres/media_tool',name)
if __name__=='__main__':unittest.main()
