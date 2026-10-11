"""Execute only our small policy expression, never the third-party JS bundle."""
import json
from pathlib import Path
import re
import shutil
import subprocess
import unittest


class BrowserStartupPolicyTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which('node'), 'Node required for JS policy semantics')
    def test_first_byte_changes_without_mutating_retry_or_total_load_defaults(self):
        go = (Path(__file__).resolve().parents[1] / 'internal/modules/emby/browser_policy.go').read_text()
        expression = re.search(r'const coldBrowserOptions = `([^`]+)`', go).group(1)
        js = r'''
const vm = require('node:vm');
const expr = JSON.parse(process.argv[1]);
const original = {maxTimeToFirstByteMs:10000,maxLoadTimeMs:120000,timeoutRetry:{maxNumRetry:4,retryDelayMs:0},errorRetry:{maxNumRetry:6,retryDelayMs:1000}};
const before = JSON.stringify(original);
const sandbox = {Hls:{DefaultConfig:{fragLoadPolicy:{default:original}}}};
vm.runInNewContext(expr+';this.result=hlsOptions;',sandbox,{timeout:1000});
if(sandbox.result.fragLoadPolicy.default.maxTimeToFirstByteMs!==60000)throw Error('first-byte budget not changed');
if(sandbox.result.fragLoadPolicy.default.maxLoadTimeMs!==120000)throw Error('total budget changed');
if(JSON.stringify(original)!==before)throw Error('global defaults mutated');
if(JSON.stringify(sandbox.result.fragLoadPolicy.default.timeoutRetry)!==JSON.stringify(original.timeoutRetry))throw Error('retry changed');
const old={Hls:{DefaultConfig:{}}};vm.runInNewContext(expr+';this.result=hlsOptions;',old,{timeout:1000});
if(old.result.fragLoadPolicy!==undefined||old.result.manifestLoadingTimeOut!==20000)throw Error('legacy fallback broke');
'''
        result = subprocess.run(['node', '-e', js, json.dumps(expression)], capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
