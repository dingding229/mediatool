import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch


class ImageSmokeCommandTests(unittest.TestCase):
    def load_script(self):
        path = Path(__file__).with_name('verify-images.py')
        spec = importlib.util.spec_from_file_location('verify_images', path)
        module = importlib.util.module_from_spec(spec)
        with patch('subprocess.run') as run, patch('subprocess.check_output') as inspect:
            spec.loader.exec_module(module)
        run.assert_not_called()
        inspect.assert_not_called()
        return module

    def test_server_role_is_isolated_and_logs_are_writable(self):
        module = self.load_script()
        for architecture in ['arm64', 'amd64']:
            with self.subTest(architecture=architecture):
                command = module.server_command(architecture, 'owned-image', 'smoke-container')
                self.assertEqual(command[-3:], ['owned-image', '-role', 'server'])
                self.assertEqual(command[command.index('--platform') + 1], 'linux/' + architecture)
                self.assertIn('APP_LOG_DIR=/tmp/media-tool-logs', command)
                self.assertIn('127.0.0.1::8080', command)
                for forbidden in ['DATABASE_URL=', 'REDIS_URL=', 'APP_SECRET_KEY=']:
                    self.assertFalse(any(arg.startswith(forbidden) for arg in command))
                self.assertNotIn('--volume', command)
                self.assertNotIn('-v', command)


if __name__ == '__main__':
    unittest.main()
