#!/usr/bin/env python3
"""Owner helper for native kurtz checks against a kept module E2E instance.

  invite  – re-grant all modules to a profile and print a fresh invitation link
  approve – approve the newest pending device for that profile

Uses only the synthetic owner credentials written by module-package-smoke.py.
"""
import argparse
import json
from pathlib import Path
import time
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('action', choices=('invite', 'approve'))
parser.add_argument('--root', type=Path, required=True)
parser.add_argument('--profile', default='Alpha')
parser.add_argument('--port', type=int, default=32596)
args = parser.parse_args()
owner = json.loads((args.root / 'owner.json').read_text())
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def jf(method, path, body=None, token=None):
    auth = 'MediaBrowser Client="Native QA", Device="Synthetic", DeviceId="native-qa", Version="0.1"' + (f', Token="{token}"' if token else '')
    req = urllib.request.Request(f'http://127.0.0.1:{args.port}{path}', data=json.dumps(body).encode() if body is not None else None,
                                 headers={'Authorization': auth, 'Content-Type': 'application/json'}, method=method)
    with opener.open(req, timeout=60) as r:
        data = r.read()
        return json.loads(data) if data else None


token = jf('POST', '/Users/AuthenticateByName', {'Username': owner['username'], 'Pw': owner['password']})['AccessToken']
profile = next(u for u in jf('GET', '/Users', None, token) if u['Name'] == args.profile)
if args.action == 'invite':
    for module in ('ai', 'photos', 'documents'):
        jf('POST', '/Mutti/Hub/admin/grants', {'module': module, 'userId': profile['Id'], 'allowed': True}, token)
    print(jf('POST', '/Mutti/Connect/invite', {}, token)['url'])
else:
    for _ in range(120):
        pending = jf('POST', '/Mutti/Connect/state', {}, token)['pending']
        if pending:
            jf('POST', '/Mutti/Connect/approve', {'pin': pending[-1]['pin'], 'userId': profile['Id']}, token)
            print('approved', pending[-1]['name'])
            break
        time.sleep(1)
    else:
        raise SystemExit('no pending device')
