#!/usr/bin/env python3
"""Authorized-host HLS startup/seek check; never export credentials or media URLs.

Use an existing non-admin Emby user's verified local session, a known item
and source. Administrator diagnostics require --allow-admin-session and are
reported as such; they never pass the non-admin permissions gate. Each run owns a unique diagnostic device/session. Stop only that
encoding. Do not mutate accounts, media identity, STRM, or client capabilities.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import re
import sqlite3
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def first_playlist_uri(data):
    text = data.decode("utf-8", errors="strict")
    if not text.lstrip().startswith("#EXTM3U"):
        raise ValueError("not a playlist")
    for line in text.splitlines():
        if line and not line.startswith("#"):
            return line.strip()
    raise ValueError("playlist has no media URI")


def segment_at(data, offset):
    text = data.decode("utf-8", errors="strict")
    if not text.lstrip().startswith("#EXTM3U"):
        raise ValueError("not a playlist")
    elapsed = 0.0
    duration = None
    for line in text.splitlines():
        if line.startswith("#EXTINF:"):
            duration = float(line.split(":", 1)[1].split(",", 1)[0])
            if not 0 < duration <= 120:
                raise ValueError("invalid segment duration")
        elif line and not line.startswith("#"):
            if duration is None:
                raise ValueError("media URI lacks duration")
            if elapsed <= offset < elapsed + duration:
                return line.strip(), elapsed
            elapsed += duration
            duration = None
    raise ValueError("seek position outside returned timeline")


def scoped_uri(base, uri, item):
    parsed = urllib.parse.urlsplit(urllib.parse.urljoin(base, uri))
    origin = urllib.parse.urlsplit(base)
    if parsed.scheme != origin.scheme or parsed.netloc != origin.netloc or parsed.username or parsed.fragment:
        raise ValueError("playlist target escaped owned origin")
    route = urllib.parse.unquote(parsed.path)
    if ".." in route.split("/") or not re.fullmatch(r"/(?:emby/)?videos/" + re.escape(item) + r"/[A-Za-z0-9_./-]+\.(?:m3u8|ts)", route, re.I):
        raise ValueError("playlist target escaped selected item")
    return urllib.parse.urlunsplit(parsed)


def playback_identity_role(user, candidate, allow_admin=False):
    """Fail closed on absent, disabled, malformed or mismatched user policies."""
    if not isinstance(user, dict) or not isinstance(user.get("Id"), str):
        return None
    if user["Id"].replace("-", "").lower() != candidate:
        return None
    policy = user.get("Policy")
    if not isinstance(policy, dict) or policy.get("EnableMediaPlayback") is not True or policy.get("IsDisabled") is not False:
        return None
    admin = policy.get("IsAdministrator")
    if admin is False:
        return "non_admin"
    if admin is True and allow_admin:
        return "admin"
    return None


def retry_interrupted_fragment(observation):
    # A retry of the same URI may start another upstream process. Never claim
    # identical URI/device/session proves identical encoding, or replay 5xx.
    return observation.get("status") is None and observation.get("interruptedOrTimedOut") is True


def diagnostic_origin(delivery):
    # Deliberately not an arbitrary operator URL: do not send tokens elsewhere.
    origins = {"gateway": "http://127.0.0.1:25015", "upstream": "http://127.0.0.1:8096"}
    if delivery not in origins:
        raise ValueError("unsupported diagnostic delivery")
    return origins[delivery]


def verified_session(root, opener, allow_admin=False):
    users = sqlite3.connect(f"file:{root}/emby/config/data/users.db?mode=ro", uri=True)
    identities = {}
    try:
        for local, guid in users.execute("SELECT Id,guid FROM LocalUsersv2"):
            if isinstance(guid, bytes) and len(guid) == 16:
                identities[local] = list(dict.fromkeys([uuid.UUID(bytes_le=guid).hex, uuid.UUID(bytes=guid).hex]))
            elif isinstance(guid, str):
                identities[local] = [uuid.UUID(guid).hex]
    finally:
        users.close()
    auth = sqlite3.connect(f"file:{root}/emby/config/data/authentication.db?mode=ro", uri=True)
    deadline = time.monotonic() + 30
    try:
        for token, local in auth.execute("SELECT AccessToken,UserId FROM Tokens_2 WHERE IsActive=1 AND UserId>0 ORDER BY DateLastActivityInt DESC LIMIT 100"):
            for candidate in identities.get(local, []):
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise RuntimeError("verified session lookup time budget exhausted")
                try:
                    req = urllib.request.Request("http://127.0.0.1:8096/Users/" + candidate, headers={"X-Emby-Token": token})
                    with opener.open(req, timeout=min(8, remaining)) as res:
                        user = json.loads(res.read(1 << 20))
                    role = playback_identity_role(user, candidate, allow_admin)
                    if role is not None:
                        return token, candidate, role
                except urllib.error.HTTPError as error:
                    error.close()
                    continue
                except Exception:
                    continue
    finally:
        auth.close()
    raise RuntimeError("no eligible verified playback-user session")


def run(args):
    root = Path(args.root).resolve()
    if not (root / ".mediatool-test-owned").is_file():
        raise RuntimeError("project ownership marker absent")
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,128}", args.item) or not re.fullmatch(r"[A-Za-z0-9_-]{1,128}", args.source):
        raise ValueError("invalid item/source")
    if not 0 <= args.offset <= 3600:
        raise ValueError("diagnostic seek outside one-hour bound")
    opener = urllib.request.build_opener(NoRedirect())
    delivery = getattr(args, "delivery", "gateway")
    gateway = diagnostic_origin(delivery)
    token, user, role = verified_session(root, opener, getattr(args, "allow_admin_session", False))
    headers = {"X-Emby-Token": token, "User-Agent": "MediaTool-HLSStartupCheck/1.0"}
    # Recheck selected item visibility before any encoder request; not admin API key.
    request = urllib.request.Request(f"http://127.0.0.1:8096/Users/{user}/Items/{args.item}?Fields=MediaSources,Path", headers=headers)
    with opener.open(request, timeout=8) as response:
        item = json.loads(response.read(1 << 20))
    if item.get("Id") != args.item or not any(source.get("Id") == args.source for source in item.get("MediaSources", [])):
        raise RuntimeError("selected source not visible to this user")
    identity = uuid.uuid4().hex
    device = "MediaTool-HLSCheck-" + identity
    session = uuid.uuid4().hex
    report = {"checkedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(), "offsetSeconds": args.offset, "ordinaryUserScopedSourceVerified": role == "non_admin", "userRole": role, "delivery": delivery, "sameEncodingOnRetryProven": False, "diagnosticDeviceOnly": True, "realPlayerDecoded": False, "rawURLsOrTokensExported": False, "requests": [], "encodingStopped": False}

    def safe_reason(value):
        return value if value in {"request_cancelled", "upstream_timeout", "upstream_closed", "upstream_unavailable"} else ""

    def request_bytes(url, kind, timeout, limit=1024, method="GET"):
        began = time.monotonic()
        req = urllib.request.Request(url, headers=headers, method=method)
        if kind == "fragment":
            req.add_header("Range", "bytes=0-1023")
        observation = {"kind": kind, "timeoutSeconds": timeout}
        try:
            with opener.open(req, timeout=timeout) as res:
                body = res.read(limit)
                observation.update(status=res.status, bytesRead=len(body), gatewayReason=safe_reason(res.headers.get("X-Media-Gateway-Reason", "")))
                if kind == "fragment":
                    observation["mpegTSSync"] = len(body) >= 376 and body[0] == 0x47 and body[188] == 0x47
                return body if res.status in (200, 206) else None
        except urllib.error.HTTPError as err:
            try:
                observation.update(status=err.code, bytesRead=0, gatewayReason=safe_reason(err.headers.get("X-Media-Gateway-Reason", "")))
            finally:
                err.close()
            return None
        except Exception:
            observation.update(status=None, interruptedOrTimedOut=True)
            return None
        finally:
            observation["elapsedMs"] = round((time.monotonic() - began) * 1000)
            report["requests"].append(observation)

    parameters = {"DeviceId": device, "MediaSourceId": args.source, "PlaySessionId": session, "api_key": token, "VideoCodec": "h264", "AudioCodec": "aac", "VideoBitrate": "20000000", "AudioBitrate": "384000", "AudioStreamIndex": "1", "TranscodingMaxAudioChannels": "2", "SegmentContainer": "ts", "MinSegments": "1", "BreakOnNonKeyFrames": "True", "StartTimeTicks": str(round(args.offset * 10000000))}
    mount = "/emby" if delivery == "gateway" else ""
    master = f"{gateway}{mount}/videos/{args.item}/master.m3u8?" + urllib.parse.urlencode(parameters)
    try:
        body = request_bytes(master, "master", 40, 2 << 20)
        if body is not None:
            leaf = scoped_uri(master, first_playlist_uri(body), args.item)
            body = request_bytes(leaf, "playlist", 40, 2 << 20)
            if body is not None:
                raw_fragment, segment_start = segment_at(body, args.offset)
                report["requestedSegmentTimelineSeconds"] = segment_start
                fragment = scoped_uri(leaf, raw_fragment, args.item)
                fragment_path = urllib.parse.urlsplit(fragment).path
                match = re.search(r"/(\d+)\.ts$", fragment_path)
                report["firstSegmentIndex"] = int(match.group(1)) if match else None
                body = request_bytes(fragment, "fragment", args.fragment_timeout)
                if body is None and retry_interrupted_fragment(report["requests"][-1]):
                    # Retry caller interruption only. An upstream 500 is a final
                    # observation, not an excuse to create a confusing retry.
                    request_bytes(fragment, "fragment", 45)
    finally:
        # This API receives the exact device AND session created above. No other
        # user's browser or Sen device is ever targeted for encoder cleanup.
        stop = f"http://127.0.0.1:8096/Videos/ActiveEncodings?" + urllib.parse.urlencode({"DeviceId": device, "PlaySessionId": session})
        request_bytes(stop, "stopOwnEncoding", 10, 1024, "DELETE")
        last = report["requests"][-1]
        report["encodingStopped"] = last.get("status") in (200, 204)
        report["startupBytesVerified"] = any(record.get("mpegTSSync") is True and record.get("status") in (200, 206) for record in report["requests"])
        destination = root / "reports" / f"hls-startup-{int(args.offset)}-{identity}.json"
        temporary = destination.with_suffix(".tmp")
        fd = os.open(temporary, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(fd, "w") as output:
            json.dump(report, output, ensure_ascii=False, indent=2)
        os.replace(temporary, destination)
        print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", default="/root/docker/mediatool")
    parser.add_argument("--item", required=True)
    parser.add_argument("--source", required=True)
    parser.add_argument("--offset", type=float, default=0)
    parser.add_argument("--fragment-timeout", type=int, default=10, choices=[10, 45])
    parser.add_argument("--delivery", choices=["gateway", "upstream"], default="gateway", help="Explicit loopback upstream control, not a playback fallback")
    parser.add_argument("--allow-admin-session", action="store_true", help="Explicitly allow an existing admin user session; cannot pass non-admin acceptance")
    args = parser.parse_args()
    try:
        run(args)
    except Exception:
        raise SystemExit("HLS check failed; credentials, URLs and raw exception omitted. Inspect private report if created.")
