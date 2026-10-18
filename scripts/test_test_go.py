"""Test isolation guards without connecting to any database or cloud service."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch


class GoIsolationTests(unittest.TestCase):
    def invoke(self, database="media_tool_test", arguments=(), require_media=False, platform="Darwin"):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            (root / "scripts").mkdir()
            shutil.copy(Path(__file__).with_name("test-go.sh"), root / "scripts/test-go.sh")
            shutil.copy(Path(__file__).with_name("test-lock.py"), root / "scripts/test-lock.py")
            (root / ".env").write_text("DATABASE_URL=postgres://fixture@localhost/business\nREDIS_URL=redis://localhost:6379/0\n")
            (root / ".env.test").write_text(f"TEST_DATABASE_URL=postgres://fixture@localhost/{database}\n")
            (root / "go").write_text("""#!/usr/bin/env python3
import json,os,sys,urllib.parse
print(json.dumps({'args':sys.argv[1:],'db':urllib.parse.urlsplit(os.environ['DATABASE_URL']).path,'redis':urllib.parse.urlsplit(os.environ['TEST_REDIS_URL']).path}))
""")
            (root / "go").chmod(0o700)
            # Keep guard-only assertions independent of the host OS. Exercise
            # the real Linux static-worker branch separately below.
            (root / "uname").write_text("#!/bin/sh\nprintf '%s\\n' " + platform + "\n")
            (root / "uname").chmod(0o700)
            # The fixture owns this flag; a CI-wide real-media requirement must
            # not change the command/output shape of isolation-only assertions.
            env = os.environ.copy()
            env["MEDIA_TOOL_REQUIRE_FFMPEG"] = "1" if require_media else "0"
            env.pop("MEDIA_TOOL_TEST_PLUGIN_UID", None)
            env.pop("MEDIA_TOOL_TEST_UID_DIR", None)
            for tool in ("ffmpeg", "ffprobe"):
                (root / tool).write_text("#!/bin/sh\nexit 0\n")
                (root / tool).chmod(0o700)
            env["PATH"] = str(root) + os.pathsep + env["PATH"]
            result = subprocess.run(["bash", str(root / "scripts/test-go.sh"), *arguments], env=env, capture_output=True, text=True)
            return result

    def test_default_database_overridden_before_migration_and_cache_isolated(self):
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        migration, test = [json.loads(line) for line in result.stdout.splitlines()]
        self.assertEqual(migration["args"], ["run", "./cmd/migrate", "-test-only", "-action", "up", "-cleanup-test-plugins"])
        self.assertEqual(test["args"], ["test", "-p", "1", "-race", "-count=1", "-timeout=20m", "./..."])
        for command in (migration, test):
            self.assertEqual(command["db"], "/media_tool_test")
            self.assertEqual(command["redis"], "/15")

    def test_business_database_refused_without_running_go(self):
        result = self.invoke(database="business")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")
        self.assertIn("only use media_tool_test", result.stderr)

    def test_packages_supported_but_parallel_override_rejected(self):
        result = self.invoke(arguments=("./internal/platform/secrets",))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout.splitlines()[1])["args"][-1], "./internal/platform/secrets")
        refused = self.invoke(arguments=("-p=8",))
        self.assertNotEqual(refused.returncode, 0)
        self.assertEqual(refused.stdout, "")

    def test_ambient_real_media_flag_is_isolated(self):
        with patch.dict(os.environ, {"MEDIA_TOOL_REQUIRE_FFMPEG": "1"}):
            result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        commands = [json.loads(line) for line in result.stdout.splitlines()]
        self.assertEqual(len(commands), 2)
        self.assertNotIn("-v", commands[1]["args"])

    def test_explicit_real_media_mode_preserves_guards_and_verbose_output(self):
        result = self.invoke(require_media=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        migration, required, test = result.stdout.splitlines()
        self.assertIn("Real media tests REQUIRED", required)
        commands = [json.loads(migration), json.loads(test)]
        self.assertEqual(commands[1]["args"], ["test", "-p", "1", "-race", "-count=1", "-v", "-timeout=20m", "./..."])
        for command in commands:
            self.assertEqual(command["db"], "/media_tool_test")
            self.assertEqual(command["redis"], "/15")

    def test_linux_builds_actual_static_worker_without_weakening_host_race(self):
        result = self.invoke(platform="Linux")
        self.assertEqual(result.returncode, 0, result.stderr)
        lines = result.stdout.splitlines()
        commands = [json.loads(line) for line in lines if line.startswith("{")]
        self.assertEqual(len(commands), 3)
        self.assertEqual(commands[1]["args"][:2], ["build", "-trimpath"])
        self.assertEqual(commands[1]["args"][-1], "./cmd/media-tool")
        self.assertEqual(commands[2]["args"], ["test", "-p", "1", "-race", "-count=1", "-timeout=20m", "./..."])
        self.assertIn("Real static app plugin-worker REQUIRED", result.stdout)
        worker = commands[1]["args"][commands[1]["args"].index("-o") + 1]
        self.assertFalse(Path(worker).parent.exists(), "scoped worker artifacts leaked")
        for command in commands:
            self.assertEqual(command["db"], "/media_tool_test")
            self.assertEqual(command["redis"], "/15")

    def test_go_major_version_binary_is_not_silently_run_as_runner(self):
        adapter = Path(__file__).with_name("test-go-plugin-exec.sh")
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            env = os.environ.copy()
            env["GITHUB_ACTIONS"] = "false"  # Must refuse before any sudo/chown.
            for name in ("plugins.test", "v2.test", "httpapi.test", "acquisition.test"):
                executable = root / name
                executable.write_text("#!/bin/sh\necho SHOULD_NOT_RUN\n")
                executable.chmod(0o700)
                result = subprocess.run(["bash", str(adapter), str(executable)], env=env, capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0, name)
                self.assertIn("Dedicated native CI identity required", result.stderr)
                self.assertNotIn("SHOULD_NOT_RUN", result.stdout)
