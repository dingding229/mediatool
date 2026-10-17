from pathlib import Path
import unittest
ROOT=Path(__file__).resolve().parent.parent
class PluginV2CI(unittest.TestCase):
    def test_release_worker_gate_is_native_static_and_nonroot(self):
        workflow=(ROOT/'.github/workflows/build.yml').read_text()
        verify=workflow.split('  verify:',1)[1].split('  publish:',1)[0]
        self.assertIn('bash scripts/test-plugin-kernel.sh',verify)
        self.assertIn('go run ./cmd/plugin-check',verify)
        self.assertIn('go run ./cmd/plugin-assets --check',verify)
        script=(ROOT/'scripts/test-plugin-kernel.sh').read_text()
        for item in ('CGO_ENABLED=0','--reuid=65532','--bounding-set=-all','--no-new-privs','MEDIA_TOOL_PLUGIN_BINARY','x86_64'):
            self.assertIn(item,script)
        self.assertNotIn('qemu',script.lower())
    def test_no_old_distribution_or_runtime_compiler(self):
        self.assertFalse((ROOT/'plugin-sdk/go').exists())
        self.assertFalse((ROOT/'examples/plugins').exists())
        self.assertNotIn('wazero',(ROOT/'go.mod').read_text())
        production='\n'.join(p.read_text() for p in (ROOT/'internal/modules/plugins/v2').rglob('*.go') if not p.name.endswith('_test.go'))
        for item in ('go build','go install','go get','http.Get(', 'exec.Command("sh"'):
            self.assertNotIn(item,production)
    def test_one_inventory_is_copied_to_frontend(self):
        docker=(ROOT/'deploy/Dockerfile').read_text()
        self.assertIn('COPY internal/modules/plugins/v2/library.json /src/internal/modules/plugins/v2/library.json',docker)
        self.assertIn('COPY --from=plugin-resources /src/web/public/',docker)
        self.assertIn('internal/modules/plugins/v2/library.json',(ROOT/'web/src/api/plugins.ts').read_text())
