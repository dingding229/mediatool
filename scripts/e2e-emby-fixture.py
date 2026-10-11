#!/usr/bin/env python3
"""Bounded loopback provider for actual save-time validation in isolated E2E.
No credentials are printed. Not a real Emby service or production record.
"""
from http.server import HTTPServer,BaseHTTPRequestHandler
import json
class Handler(BaseHTTPRequestHandler):
 def do_GET(self):
  if self.path=='/Users' and self.headers.get('X-Emby-Token')=='PRIVATE-EMBY-ECHO-FIXTURE':
   status,body=200,[{'Id':'owned-fixture-user','Name':'Owned E2E fixture'}]
  else:status,body=401,{'error':'owned fixture authentication required'}
  raw=json.dumps(body).encode();self.send_response(status);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw)
 def log_message(self,*args):pass
HTTPServer(('127.0.0.1',18086),Handler).serve_forever()
