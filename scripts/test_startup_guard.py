import json
from pathlib import Path
import shutil
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]

class BoundedStartupGuardTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which('node'), 'Node is required for startup callback semantics')
    def test_transient_wait_permission_fatal_abort_destroy_and_deadline(self):
        result = subprocess.run(['node', str(ROOT / 'scripts/browser_startup_guard_harness.cjs'),
                                 str(ROOT / 'internal/modules/emby/browser_startup_guard.js')],
                                capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0, 'owned bounded-startup harness failed')
        report = json.loads(result.stdout)
        self.assertGreaterEqual(report['cases'], 30)
        self.assertEqual(report['startupBudgetMs'], 90000)
        self.assertEqual(report['manualRetries'], 0)
        self.assertFalse(report['permissionOrFatalBypass'])
