#!/usr/bin/env python3
"""Serialize Go/E2E suites that mutate the shared, isolated test database."""
import fcntl,os,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parent.parent
lockdir=root/'data';lockdir.mkdir(exist_ok=True)
with (lockdir/'.isolated-test.lock').open('a') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX)
 env=os.environ.copy();env['MEDIA_TOOL_TEST_LOCKED']='1'
 raise SystemExit(subprocess.call(['bash',*sys.argv[1:]],cwd=root,env=env))
