import subprocess, unittest
from pathlib import Path
class PluginSupportFreezeTests(unittest.TestCase):
    def test_public_contract_and_scope_matrix_is_current(self):
        root=Path(__file__).resolve().parent.parent
        subprocess.run(['python3',str(root/'scripts/plugin-support-matrix.py'),'--check'],cwd=root,check=True)
    def test_failure_recording_is_safe_and_playback_version_has_one_source(self):
        root=Path(__file__).resolve().parent.parent
        s=(root/'internal/platform/httpapi/plugins_v2.go').read_text()
        body=s.split('func (a *API) pluginOperationRejected(',1)[1].split('\nfunc ',1)[0]
        self.assertNotIn('.Error()',body);self.assertNotIn('in.Config',body);self.assertNotIn('OperationKey',body)
        for name in ['api.go','external_api.go']:
            self.assertIn('"version": buildinfo.Version',(root/'internal/platform/httpapi'/name).read_text())
