import importlib.util
from pathlib import Path
import unittest
import contextlib
import io
import json
import sqlite3
import tempfile
import uuid

module_spec = importlib.util.spec_from_file_location('hls_check', Path(__file__).with_name('remote-hls-check.py'))
hls = importlib.util.module_from_spec(module_spec)
module_spec.loader.exec_module(hls)


class PlaylistScopeTests(unittest.TestCase):
    def test_master_and_fragment_keep_owned_origin_item_and_query(self):
        base = 'http://127.0.0.1:25015/emby/videos/11/master.m3u8?api_key=fixture'
        leaf = hls.scoped_uri(base, hls.first_playlist_uri(b'#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nmain.m3u8?api_key=fixture\n'), '11')
        self.assertEqual(leaf, 'http://127.0.0.1:25015/emby/videos/11/main.m3u8?api_key=fixture')
        fragment = hls.scoped_uri(leaf, 'hls1/main/170.ts?api_key=fixture', '11')
        self.assertIn('/videos/11/hls1/main/170.ts?', fragment)

    def test_foreign_origin_item_traversal_userinfo_and_nonplaylist_refused(self):
        base = 'http://127.0.0.1:25015/emby/videos/11/master.m3u8'
        for uri in ['https://foreign.example/video.ts', '//other.invalid/video.ts', '/emby/videos/12/hls1/main/0.ts', '/emby/videos/11/../12/0.ts', '/emby/videos/11/%2e%2e/12/0.ts', 'http://user@127.0.0.1:25015/emby/videos/11/0.ts']:
            with self.subTest(uri=uri), self.assertRaises(ValueError):
                hls.scoped_uri(base, uri, '11')
        with self.assertRaises(ValueError):
            hls.first_playlist_uri(b'<html>provider error</html>')

    def test_seek_selects_requested_segment_not_first_uri(self):
        media = b"#EXTM3U\n#EXTINF:3,\nhls1/main/0.ts\n#EXTINF:3,\nhls1/main/1.ts\n#EXTINF:3,\nhls1/main/2.ts\n"
        self.assertEqual(hls.segment_at(media, 0), ('hls1/main/0.ts', 0.0))
        self.assertEqual(hls.segment_at(media, 7), ('hls1/main/2.ts', 6.0))
        with self.assertRaises(ValueError):
            hls.segment_at(media, 10)


class VerifiedIdentityTests(unittest.TestCase):
    def user(self, **overrides):
        policy = {"EnableMediaPlayback": True, "IsAdministrator": False, "IsDisabled": False}
        policy.update(overrides)
        return {"Id": "a" * 32, "Policy": policy}

    def test_non_admin_default_and_explicit_admin_are_distinct(self):
        self.assertEqual(hls.playback_identity_role(self.user(), "a" * 32), "non_admin")
        admin = self.user(IsAdministrator=True)
        self.assertIsNone(hls.playback_identity_role(admin, "a" * 32))
        self.assertEqual(hls.playback_identity_role(admin, "a" * 32, True), "admin")

    def test_missing_malformed_disabled_denied_or_mismatched_refused(self):
        values = [None, {}, {"Id": 1}, {"Id": "a" * 32, "Policy": []}, self.user(IsDisabled=True), self.user(EnableMediaPlayback=False), self.user(IsAdministrator=0), self.user(IsDisabled=0)]
        for user in values:
            with self.subTest(user=user):
                self.assertIsNone(hls.playback_identity_role(user, "a" * 32, True))
        for key in ["EnableMediaPlayback", "IsAdministrator", "IsDisabled"]:
            user = self.user()
            del user["Policy"][key]
            self.assertIsNone(hls.playback_identity_role(user, "a" * 32, True))
        self.assertIsNone(hls.playback_identity_role(self.user(), "b" * 32, True))

    def test_session_selection_skips_admin_unless_explicit(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            data = root / "emby/config/data"
            data.mkdir(parents=True)
            identities = [(1, uuid.UUID(int=1).hex), (2, uuid.UUID(int=2).hex)]
            with contextlib.closing(sqlite3.connect(data / "users.db")) as conn, conn:
                conn.execute("CREATE TABLE LocalUsersv2 (Id INTEGER, guid TEXT)")
                conn.executemany("INSERT INTO LocalUsersv2 VALUES (?,?)", identities)
            with contextlib.closing(sqlite3.connect(data / "authentication.db")) as conn, conn:
                conn.execute("CREATE TABLE Tokens_2 (AccessToken TEXT, UserId INTEGER, IsActive INTEGER, DateLastActivityInt INTEGER)")
                conn.executemany("INSERT INTO Tokens_2 VALUES (?,?,1,?)", [("fixture-admin", 1, 20), ("fixture-viewer", 2, 10)])

            class Opener:
                def open(self, req, timeout):
                    user_id = req.full_url.rsplit("/", 1)[-1]
                    return contextlib.closing(io.BytesIO(json.dumps({"Id": user_id, "Policy": {"EnableMediaPlayback": True, "IsDisabled": False, "IsAdministrator": user_id == identities[0][1]}}).encode()))

            self.assertEqual(hls.verified_session(root, Opener()), ("fixture-viewer", identities[1][1], "non_admin"))
            self.assertEqual(hls.verified_session(root, Opener(), True), ("fixture-admin", identities[0][1], "admin"))
            with contextlib.closing(sqlite3.connect(data / "authentication.db")) as conn, conn:
                conn.execute("DELETE FROM Tokens_2 WHERE UserId=2")
            with self.assertRaises(RuntimeError):
                hls.verified_session(root, Opener())
            from unittest import mock
            with mock.patch.object(hls.time, "monotonic", side_effect=[0, 30]), self.assertRaises(RuntimeError):
                hls.verified_session(root, Opener(), True)

    def test_retry_is_only_caller_interruption_not_upstream_5xx(self):
        self.assertTrue(hls.retry_interrupted_fragment({"status": None, "interruptedOrTimedOut": True}))
        for observation in [{}, {"status": 500}, {"status": 502}, {"status": 200}, {"status": 500, "interruptedOrTimedOut": True}]:
            self.assertFalse(hls.retry_interrupted_fragment(observation))

    def test_delivery_control_never_accepts_arbitrary_origins(self):
        self.assertEqual(hls.diagnostic_origin("gateway"), "http://127.0.0.1:25015")
        self.assertEqual(hls.diagnostic_origin("upstream"), "http://127.0.0.1:8096")
        for value in ["http://remote.example", "http://127.0.0.1:9999", "", None]:
            with self.assertRaises(ValueError):
                hls.diagnostic_origin(value)


class RunEvidenceTests(unittest.TestCase):
    def test_admin_upstream_control_reports_role_and_does_not_replay_500(self):
        from types import SimpleNamespace
        from unittest import mock
        import urllib.error
        import urllib.parse
        from email.message import Message

        calls = []
        token = 'fixture-token-not-in-reports'

        class Response(io.BytesIO):
            def __init__(self, body=b'', status=200):
                super().__init__(body)
                self.status = status
                self.headers = Message()

        class Opener:
            def open(self, req, timeout):
                calls.append((req.get_method(), urllib.parse.urlsplit(req.full_url).path, timeout))
                path = calls[-1][1]
                if path.startswith('/Users/'):
                    return Response(json.dumps({'Id': '11', 'MediaSources': [{'Id': 'mediasource_11'}]}).encode())
                if path.endswith('/master.m3u8'):
                    return Response(b'#EXTM3U\nmain.m3u8\n')
                if path.endswith('/main.m3u8'):
                    return Response(('#EXTM3U\n' + ''.join(f'#EXTINF:6,\nhls1/main/{index}.ts\n' for index in range(86))).encode())
                if path.endswith('.ts'):
                    raise urllib.error.HTTPError(req.full_url, 500, 'fixture-private-error', Message(), None)
                if req.get_method() == 'DELETE':
                    return Response(status=204)
                raise AssertionError('unexpected fixture route')

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / '.mediatool-test-owned').touch()
            (root / 'reports').mkdir()
            args = SimpleNamespace(root=temporary, item='11', source='mediasource_11', offset=510, fragment_timeout=45, delivery='upstream', allow_admin_session=True)
            out = io.StringIO()
            with mock.patch.object(hls, 'verified_session', return_value=(token, 'a' * 32, 'admin')), mock.patch.object(hls.urllib.request, 'build_opener', return_value=Opener()), contextlib.redirect_stdout(out):
                hls.run(args)
            report = json.loads(out.getvalue())
            self.assertEqual(report['userRole'], 'admin')
            self.assertFalse(report['ordinaryUserScopedSourceVerified'])
            self.assertEqual(report['delivery'], 'upstream')
            self.assertIn(('GET', '/videos/11/master.m3u8', 40), calls)
            self.assertEqual(report['firstSegmentIndex'], 85)
            self.assertFalse(report['sameEncodingOnRetryProven'])
            self.assertFalse(report['startupBytesVerified'])
            self.assertTrue(report['encodingStopped'])
            self.assertEqual(len([x for x in calls if x[1].endswith('.ts')]), 1)
            self.assertNotIn(token, out.getvalue())
            self.assertNotIn('fixture-private-error', out.getvalue())
            self.assertTrue(all(file.stat().st_mode & 0o777 == 0o600 for file in (root / 'reports').glob('*.json')))
