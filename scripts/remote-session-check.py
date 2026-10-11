#!/usr/bin/env python3
"""Run on the authorized Emby host; never export authentication DBs/tokens.

Read-only SQLite access establishes existing token -> local user -> GUID
associations. Send credentials to a privileged maintenance binary via stdin,
which saves only endpoint-scoped SHA256 identity hints and rechecks permissions.
No login/password/account changes. Limit diagnostics to this Compose project.
"""
import argparse
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import urllib.error
import urllib.request
import uuid


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def run(args):
    root = Path(args.root).resolve()
    if not (root / '.mediatool-test-owned').is_file():
        raise RuntimeError('deployment ownership marker absent')
    opener = urllib.request.build_opener(NoRedirect())
    users = sqlite3.connect(f'file:{root}/emby/config/data/users.db?mode=ro', uri=True)
    identities = {}
    for local_id, guid in users.execute('SELECT Id,guid FROM LocalUsersv2'):
        # Emby uses a .NET GUID; support both byte serializations by confirming
        # the canonical GUID against the actual server, not client UserId.
        if isinstance(guid, bytes) and len(guid) == 16:
            identities[local_id] = list(dict.fromkeys([uuid.UUID(bytes_le=guid).hex, uuid.UUID(bytes=guid).hex]))
        elif isinstance(guid, str):
            identities[local_id] = [uuid.UUID(guid).hex]
    users.close()
    auth = sqlite3.connect(f'file:{root}/emby/config/data/authentication.db?mode=ro', uri=True)
    sessions = []
    for token, local_id in auth.execute('SELECT AccessToken,UserId FROM Tokens_2 WHERE IsActive=1 AND UserId>0 ORDER BY DateLastActivityInt DESC LIMIT 100'):
        for candidate in identities.get(local_id, []):
            request = urllib.request.Request('http://127.0.0.1:8096/Users/' + candidate,
                                             headers={'X-Emby-Token': token, 'User-Agent': 'MediaTool-GatewayCheck'})
            try:
                with opener.open(request, timeout=10) as response:
                    user = json.loads(response.read(1 << 20))
                if user.get('Id', '').replace('-', '').lower() != candidate:
                    continue
                sessions.append({'Token': token, 'UserID': user['Id']})
                break
            except (urllib.error.HTTPError, urllib.error.URLError, ValueError):
                continue
    auth.close()
    if not sessions:
        raise RuntimeError('no verified active local-user session')
    server = subprocess.check_output(['docker', 'compose', '--env-file', '.env', 'ps', '-q', 'server'], cwd=root, text=True).strip()
    if not server:
        raise RuntimeError('server container absent')
    payload = json.dumps({'Sessions': sessions, 'ItemID': args.item, 'SourceID': args.source, 'ClientAgents': args.client_agent}).encode()
    command = ['docker', 'exec', '-i', server, '/tmp/gateway-session-smoke']
    if args.bind:
        command.append('--bind-existing-sessions')
    if args.ephemeral_client_link:
        command.append('--ephemeral-client-link')
    result = subprocess.run(command, input=payload, cwd=root, capture_output=True, timeout=125)
    if result.returncode:
        raise RuntimeError('credential-redacted gateway diagnostic did not succeed')
    if args.ephemeral_client_link:
        # Captured only by a trusted diagnostic process; no disk/report write.
        sys.stdout.buffer.write(result.stdout)
        return
    report = json.loads(result.stdout)
    report['associationSource'] = 'read-only-local-Emby-authentication-database'
    destination = root / 'reports' / 'gateway-user-session-check.json'
    temporary = destination.with_suffix('.json.tmp')
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as stream:
        json.dump(report, stream, ensure_ascii=False, indent=2)
    os.replace(temporary, destination)
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', default='/root/docker/mediatool')
    parser.add_argument('--item', required=True)
    parser.add_argument('--source', required=True)
    parser.add_argument('--ephemeral-client-link', action='store_true', help='private stdout pipe only; never print or log this capability')
    parser.add_argument('--client-agent', action='append', default=[])
    parser.add_argument('--bind', action='store_true', help='explicitly backfill verified pre-existing session identity hints')
    try:
        run(parser.parse_args())
    except Exception:
        print('User-session diagnostic failed; raw errors and credentials omitted.', file=sys.stderr)
        sys.exit(1)
