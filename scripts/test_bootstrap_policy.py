import json
from pathlib import Path
import re
import shutil
import subprocess
import unittest
from urllib.parse import parse_qs, urlsplit

SOURCE = (Path(__file__).resolve().parents[1] / 'internal/modules/emby/browser_policy.go').read_text()

def constant(name):
    match = re.search(r'const ' + re.escape(name) + r' = `([^`]+)`', SOURCE)
    if not match:
        raise AssertionError('own policy constant missing')
    return match.group(1)

class BootstrapPolicyTests(unittest.TestCase):
    def test_index_loader_url_retains_the_real_version_and_adds_one_policy_key(self):
        tag = constant('revisedLoaderTag')
        address = re.search(r'src="([^"]+)"', tag).group(1)
        query = parse_qs(urlsplit(address).query)
        self.assertEqual(query['v'], ['4.10.1.0'])
        self.assertEqual(query['mt_hls_startup'], ['2'])
        self.assertNotIn('dashboardVersion', tag)

    def test_loader_cache_expression_changes_only_the_asset_cache_key(self):
        if not shutil.which('node'):
            self.skipTest('Node required for executing the own cache expression')
        expression = constant('revisedLoaderCache')
        code = '''
const expression = JSON.parse(process.argv[1]);
const apply = new Function('docElem', 'appMode', expression + ';return globalThis.urlCacheParam;');
globalThis.dashboardVersion = '4.10.1.0';
const first = apply('4.10.1.0', '');
const second = apply('4.10.1.0', '');
if(first !== 'v=4.10.1.0&mt_hls_startup=2' || second !== first) throw new Error('revision duplicated or version changed');
if(globalThis.dashboardVersion !== '4.10.1.0') throw new Error('real dashboard version changed');
const previous = Date.now; Date.now = () => 12345;
const fallback = apply('', ''); Date.now = previous;
if(fallback !== 'v=12345') throw new Error('unversioned fallback changed');
globalThis.urlCacheParam = 'native-preserved';
if(apply('', 'android') !== 'native-preserved') throw new Error('native fallback changed');
console.log('own-bootstrap-expression-ok');
'''
        result = subprocess.run(['node', '-e', code, json.dumps(expression)], capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, 'own cache expression evaluation failed')
        self.assertEqual(result.stdout.strip(), 'own-bootstrap-expression-ok')
