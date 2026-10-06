#!/usr/bin/env python3
"""End-to-end module test of the packaged Mac server through real paired devices.

Creates a new synthetic Mutti instance from the built Mutti.app (never the
user's data), the isolated Immich/Paperless test environment from
module-testenv.py, two devices paired over the encrypted Connect tunnel
(mutti-testdevice) and the managed, network-locked local AI engine.
Owner actions use the Jellyfin bridge exactly like the web management.

  module-package-smoke.py --app build/macos/osx-arm64/Mutti.app \
      --root build/module-e2e-NEW --testenv build/module-testenv/private.json \
      [--model qwen3.5:4b [--expect-qualified]]

Since P0 no deployment is qualified, so by default the AI must stay locked
(qualification_required) while manual content access keeps working (P1).
--expect-qualified runs the positive AI checks AT-03..AT-12; it can only pass
once a trusted qualification for exactly this deployment exists (P2).
"""
import argparse
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument('--app', type=Path, required=True)
parser.add_argument('--root', type=Path, required=True)
parser.add_argument('--testenv', type=Path, required=True)
parser.add_argument('--model', help='also install and select this pinned model')
parser.add_argument('--expect-qualified', action='store_true', help='run the positive AI path (requires real qualification)')
parser.add_argument('--model-source', choices=('pull', 'adopt'), default='pull')
parser.add_argument('--language', default='de', help='answer language sent with AI messages (the AT prompts are German)')
parser.add_argument('--adopt-from', help='local engine store (models dir) to adopt from; digests are verified')
parser.add_argument('--keep', action='store_true', help='keep the instance running for native client checks')
parser.add_argument('--setup-only', action='store_true', help='set up the synthetic instance, write fixture.json for qualification and keep it running')
args = parser.parse_args()
repo = Path(__file__).resolve().parents[2]
root = args.root.resolve()
if root.exists():
    raise SystemExit('Use a new directory; no existing data is accepted')
root.mkdir(parents=True, mode=0o700)
resources = args.app.resolve() / 'Contents/Resources'
env_data = json.loads(args.testenv.read_text())
PORTS = {'hub': 32593, 'manager': 32594, 'connect': 32595, 'jellyfin': 32596, 'broker': 32600}
for port in PORTS.values():
    with socket.socket() as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        probe.bind(('127.0.0.1', port))
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
report = {'started': time.time(), 'checks': [], 'model': args.model, 'app': str(args.app)}
if args.setup_only:
    args.keep = True


class SetupDone(Exception):
    pass
processes, logs = [], []


def check(label, condition, detail=None):
    if not condition:
        raise AssertionError(label + (f' ({detail})' if detail is not None else ''))
    report['checks'].append(label)
    print('PASS:', label, flush=True)


def start(name, argv, env=None):
    log = (root / f'{name}.log').open('w')
    logs.append(log)
    p = subprocess.Popen(argv, env=env, stdout=log, stderr=log, start_new_session=True)
    processes.append(p)
    return p


def http(method, url, body=None, headers=None, timeout=60, raw=False):
    data = None
    headers = dict(headers or {})
    if isinstance(body, (bytes, bytearray)):
        data = bytes(body)
    elif body is not None:
        data = json.dumps(body).encode()
        headers.setdefault('Content-Type', 'application/json')
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with opener.open(req, timeout=timeout) as r:
            content = r.read()
            return r.status, (content if raw else (json.loads(content) if content else None))
    except urllib.error.HTTPError as e:
        content = e.read()
        try:
            return e.code, json.loads(content)
        except ValueError:
            return e.code, content


def jf(method, path, body=None, token=None, **kw):
    auth = 'MediaBrowser Client="Module E2E", Device="Synthetic", DeviceId="module-e2e", Version="0.1"'
    if token:
        auth += f', Token="{token}"'
    return http(method, f'http://127.0.0.1:{PORTS["jellyfin"]}{path}', body, {'Authorization': auth}, **kw)


def ok(result, label):
    status, body = result
    if status not in (200, 201, 202, 204):
        raise AssertionError(f'{label}: HTTP {status} {str(body)[:300]}')
    return body


def hub_admin(op, body=None):
    return jf('POST', f'/Mutti/Hub/{op}', body if body is not None else {}, owner)


def clip(path, seconds):
    subprocess.run([str(resources / 'ffmpeg/ffmpeg'), '-v', 'error', '-f', 'lavfi', '-i', f'testsrc2=size=320x180:rate=10', '-t', str(seconds),
                    '-c:v', 'libx264', '-preset', 'ultrafast', '-pix_fmt', 'yuv420p', '-an', str(path)], check=True)


class Device:
    def __init__(self, name, user_id):
        self.name, self.user_id = name, user_id
        self.creds = root / f'{name}-credentials.json'

    def pair(self):
        invite = ok(jf('POST', '/Mutti/Connect/invite', {}, owner), 'invite')['url']
        self.proc = subprocess.Popen([str(repo / 'build/mutti-testdevice'), '--invite', invite, '--name', self.name, '--credentials', str(self.creds)],
                                     stdout=subprocess.PIPE, stderr=(root / f'{self.name}.log').open('w'), text=True,
                                     env=dict(os.environ, MUTTI_ICE_INTERFACES='lo0'), start_new_session=True)
        processes.append(self.proc)
        assert json.loads(self.proc.stdout.readline())['state'] == 'pairing'
        for _ in range(60):
            pending = ok(jf('POST', '/Mutti/Connect/state', {}, owner), 'state')['pending']
            mine = [p for p in pending if p['name'] == self.name]
            if mine:
                ok(jf('POST', '/Mutti/Connect/approve', {'pin': mine[0]['pin'], 'userId': self.user_id}, owner), 'approve')
                break
            time.sleep(0.5)
        else:
            raise AssertionError('pairing request never arrived')
        ready = json.loads(self.proc.stdout.readline())
        if ready.get('state') != 'ready':
            raise AssertionError(f'pairing failed: {ready}')
        self.base, self.pin = ready['gateway'], ready['pin']
        return self

    def call(self, method, path, body=None, headers=None, raw=False, timeout=120):
        if path.endswith('/messages') and isinstance(body, dict):
            body = dict(body, language=body.get('language', args.language))
        return http(method, self.base + '/mutti/hub/v1/' + path, body, headers, timeout=timeout, raw=raw)

    def media(self, path):
        return http('GET', self.base + path, raw=True)

    def events(self, run, conversation, stop=None, after=0, timeout=240):
        url = f'{self.base}/mutti/hub/v1/ai/runs/{run}/events?conversation={conversation}&after={after}'
        req = urllib.request.Request(url, headers={'Accept': 'text/event-stream'})
        out, cur = [], {}
        with opener.open(req, timeout=timeout) as r:
            for line in r:
                line = line.decode().rstrip('\n')
                if line.startswith('id: '):
                    cur['id'] = int(line[4:])
                elif line.startswith('event: '):
                    cur['type'] = line[7:]
                elif line.startswith('data: '):
                    cur['data'] = json.loads(line[6:])
                elif line == '' and 'type' in cur:
                    out.append(cur)
                    if stop and stop(cur):
                        return out
                    if cur['type'] == 'done':
                        return out
                    cur = {}
        return out

    def ask(self, conversation, text):
        status, started = self.call('POST', f'ai/conversations/{conversation}/messages', {'text': text, 'idempotencyKey': secrets.token_hex(8)})
        if status != 202:
            raise AssertionError(f'ask {status} {started}')
        events = self.events(started['run']['id'], conversation)
        return started['run']['id'], events, events[-1]['data']


try:
    media, private = root / 'media/Filme', root / 'media/Privat'
    media.mkdir(parents=True)
    private.mkdir(parents=True)
    for name, seconds in (('Nordlicht (2024)', 84), ('Sommer am See (2023)', 95), ('Lange Reise (2022)', 125), ('Schon gesehen (2023)', 88)):
        clip(media / f'{name}.mp4', seconds)
    clip(private / 'Privater Film B (2024).mp4', 92)
    start('broker', [str(resources / 'connect/mutti-connect'), '--mode', 'broker', '--listen', f'127.0.0.1:{PORTS["broker"]}', '--stun-listen', '127.0.0.1:0'])
    manager = start('manager', [str(resources / 'migrate/mutti-migrate'), '--root', str(root / 'server'), '--server', str(resources / 'server/jellyfin'),
                                '--web', str(resources / 'web'), '--ffmpeg', str(resources / 'ffmpeg/ffmpeg'), '--intro-skipper', str(resources / 'intro-skipper'),
                                '--connect', str(resources / 'connect/mutti-connect'), '--hub', str(resources / 'hub/mutti-hub'),
                                '--hub-listen', f'127.0.0.1:{PORTS["hub"]}', '--ollama', str(resources / 'ai-engine/ollama'),
                                '--listen', f'127.0.0.1:{PORTS["manager"]}', '--origin', f'http://127.0.0.1:{PORTS["manager"]}',
                                '--backend', f'http://127.0.0.1:{PORTS["jellyfin"]}', '--target-origin', f'http://127.0.0.1:{PORTS["jellyfin"]}',
                                '--connect-listen', f'127.0.0.1:{PORTS["connect"]}', '--connect-origin', f'http://127.0.0.1:{PORTS["connect"]}'],
                    dict(os.environ, MUTTI_SIGNAL_URL=f'http://127.0.0.1:{PORTS["broker"]}', MUTTI_STUN_URL='',
                         # Native checks with --keep may add the LAN interface (e.g. lo0,en0) for a simulator client.
                         MUTTI_ICE_INTERFACES=os.environ.get('MUTTI_E2E_ICE', 'lo0')))
    for _ in range(120):
        if manager.poll() is not None:
            raise AssertionError(f'manager exited with {manager.returncode}; see manager.log')
        try:
            if jf('GET', '/Startup/User')[0] == 200:
                break
        except urllib.error.URLError:
            pass  # still starting
        time.sleep(1)
    else:
        raise AssertionError('server did not start')
    password = secrets.token_urlsafe(24)
    ok(jf('POST', '/Startup/Configuration', {'ServerName': 'Mutti Module E2E', 'UICulture': 'de', 'MetadataCountryCode': 'DE', 'PreferredMetadataLanguage': 'de'}), 'config')
    ok(jf('POST', '/Startup/User', {'Name': 'Testbesitzer', 'Password': password}), 'owner')
    ok(jf('POST', '/Startup/RemoteAccess', {'EnableRemoteAccess': False}), 'remote')
    owner = ok(jf('POST', '/Users/AuthenticateByName', {'Username': 'Testbesitzer', 'Pw': password}), 'login')['AccessToken']
    ok(jf('POST', '/Startup/Complete', {}, owner), 'complete')
    (root / 'owner.json').write_text(json.dumps({'username': 'Testbesitzer', 'password': password}))
    (root / 'owner.json').chmod(0o600)
    users, passwords = {}, {}
    for name in ('Alpha', 'Beta'):
        passwords[name] = secrets.token_urlsafe(24)
        users[name] = ok(jf('POST', '/Users/New', {'Name': name, 'Password': passwords[name]}, owner), 'profile')['Id']
    options = {'LibraryOptions': {'EnableRealtimeMonitor': False, 'EnableInternetProviders': False, 'SaveLocalMetadata': False,
                                  'TypeOptions': [{'Type': 'Movie', 'MetadataFetchers': [], 'ImageFetchers': []}]}}
    for name, path in (('Filme', media), ('Privat', private)):
        query = urllib.parse.urlencode({'name': name, 'collectionType': 'movies', 'paths': str(path), 'refreshLibrary': 'true'})
        ok(jf('POST', f'/Library/VirtualFolders?{query}', options, owner), 'library')
    folders = {f['Name']: f['ItemId'] for f in ok(jf('GET', '/Library/VirtualFolders', None, owner), 'folders')}
    for attempt in range(120):
        items = ok(jf('GET', '/Items?IncludeItemTypes=Movie&Recursive=true&Fields=Path', None, owner), 'items')['Items']
        if len(items) == 5:
            break
        if attempt % 15 == 14:
            # Two libraries added back to back can coalesce into one scan.
            ok(jf('POST', '/Library/Refresh', None, owner), 'refresh')
        time.sleep(2)
    else:
        raise AssertionError('library scan incomplete')
    movie = {i['Name']: i['Id'] for i in items}
    check('Synthetic movie clips imported', set(movie) >= {'Nordlicht', 'Sommer am See', 'Lange Reise', 'Schon gesehen', 'Privater Film B'}, list(movie))
    for name, folder_ids in (('Alpha', [folders['Filme']]), ('Beta', [folders['Filme'], folders['Privat']])):
        policy = ok(jf('GET', f'/Users/{users[name]}', None, owner), 'user')['Policy']
        policy.update(EnableAllFolders=False, EnabledFolders=folder_ids)
        ok(jf('POST', f'/Users/{users[name]}/Policy', policy, owner), 'policy')
    ok(jf('POST', f'/UserPlayedItems/{movie["Schon gesehen"]}?userId={users["Alpha"]}', None, owner), 'played')

    # Module service is present and owner-only.
    status, _ = jf('POST', '/Mutti/Hub/admin/state', {}, None)
    check('Anonymous module administration is denied', status == 401, status)
    management = ok(jf('GET', '/Mutti/Management', None, owner), 'management')
    check('Package announces the module service', management.get('modulesAvailable') is True, management)
    for _ in range(60):
        status, state = hub_admin('admin/state')
        if status == 200:
            break
        time.sleep(1)
    check('Module service started after setup', status == 200, status)

    # Photos and documents: profile-bound accounts, never administrator keys.
    ok(hub_admin('admin/service/photos', {'url': env_data['photos_url']}), 'photos service')
    ok(hub_admin('admin/service/documents', {'url': env_data['documents_url']}), 'documents service')
    status, body = hub_admin('admin/link/photos', {'userId': users['Alpha'], 'account': env_data['photos_admin']['email'], 'password': env_data['photos_admin']['password']})
    check('Immich administrator account is rejected for a profile', status == 400 and body.get('code') == 'admin_account', body)
    for name, account in (('Alpha', 'alpha'), ('Beta', 'beta')):
        a = env_data['accounts']['photos'][account]
        ok(hub_admin('admin/link/photos', {'userId': users[name], 'account': a['email'], 'password': a['password']}), 'link photos')
        d = env_data['accounts']['documents'][account]
        ok(hub_admin('admin/link/documents', {'userId': users[name], 'account': d['username'], 'password': d['password']}), 'link documents')
        for module in ('photos', 'documents', 'ai'):
            ok(hub_admin('admin/grants', {'module': module, 'userId': users[name], 'allowed': True}), 'grant')
    ok(hub_admin('admin/enable/photos', {'enabled': True}), 'enable photos')
    ok(hub_admin('admin/enable/documents', {'enabled': True}), 'enable documents')
    state = ok(hub_admin('admin/state'), 'state')
    check('Owner state contains no service secrets or passwords', 'secret' not in json.dumps(state) and env_data['accounts']['photos']['alpha']['password'] not in json.dumps(state))
    if args.setup_only:
        # Synthetic credentials and object IDs for mutti-hub qualify (0600).
        norm = lambda i: i.replace('-', '').lower()
        objects = {f'movie:{name}': norm(i) for name, i in movie.items()}
        objects.update({f'document:{k}': str(v) for k, v in env_data['documents_fixture'].items()})
        objects.update({f'photo:{k}': v for k, v in env_data['photos_fixture'].items()})
        fixture = {'jellyfin': f'http://127.0.0.1:{PORTS["jellyfin"]}', 'hubState': str(root / 'server' / 'hub'),
                   'profiles': {'alpha': {'name': 'Alpha', 'password': passwords['Alpha']}, 'beta': {'name': 'Beta', 'password': passwords['Beta']}},
                   'objects': objects}
        (root / 'fixture.json').write_text(json.dumps(fixture, indent=1))
        (root / 'fixture.json').chmod(0o600)
        report['passed'] = True
        print('Setup ready:', root / 'fixture.json', flush=True)
        raise SetupDone()

    # Local AI: managed engine with OS network lock and a digest-pinned model.
    if args.expect_qualified and not args.model:
        raise SystemExit('--expect-qualified needs --model')
    if args.model:
        request = {'model': args.model}
        if args.model_source == 'adopt' and args.adopt_from:
            request['source'] = str(Path(args.adopt_from).resolve())
        ok(hub_admin(f'admin/ai/models/{args.model_source}', request), 'model download')
        t0 = time.time()
        while True:
            engine = ok(hub_admin('admin/state'), 'state')['modules']['ai']['ai']['engine']
            download = engine.get('download') or {}
            if download.get('status') in ('done', 'failed'):
                break
            if time.time() - t0 > 3600:
                raise AssertionError('model download timeout')
            time.sleep(5)
        check('Model download verified against the pinned digest', download.get('status') == 'done', download)
        report['modelSeconds'] = round(time.time() - t0, 1)
        for _ in range(60):
            status, body = hub_admin('admin/ai/models/select', {'model': args.model})
            if status == 200:
                break
            time.sleep(2)
        check('Pinned model selected', status == 200, body)
        ok(hub_admin('admin/enable/ai', {'enabled': True}), 'enable ai')
        engine = ok(hub_admin('admin/state'), 'state')['modules']['ai']['ai']['engine']
        check('Managed engine is confined by the macOS network sandbox', engine['networkLock'] == 'verified' and engine['state'] == 'ready', engine)

    # Two paired devices through the real encrypted tunnel.
    alpha = Device('Alpha-Testgeraet', users['Alpha']).pair()
    beta = Device('Beta-Testgeraet', users['Beta']).pair()
    ai_ready = 'ready' if args.expect_qualified else None
    for _ in range(30):
        caps = alpha.call('GET', f'capabilities?language={args.language}')[1]
        if all(caps['modules'][m]['state'] == 'ready' for m in ('photos', 'documents', 'content')) and (ai_ready is None or caps['modules']['ai']['state'] == 'ready'):
            break
        time.sleep(2)
    check('AT-02 Paired profile sees exactly its allowed modules', all(caps['modules'][m]['state'] == 'ready' for m in ('photos', 'documents', 'content')), caps)
    if not args.expect_qualified:
        expected = 'qualification_required' if args.model else 'not_configured'
        check(f'P0 AI stays locked without qualification ({expected})', caps['modules']['ai']['state'] == expected and 'chat' not in caps['modules']['ai']['actions'], caps['modules']['ai'])
        if args.model:
            status, conv = alpha.call('POST', 'ai/conversations', {'title': ''})
            status, body = alpha.call('POST', f'ai/conversations/{conv["id"]}/messages', {'text': 'Hallo', 'idempotencyKey': 'locked-1'})
            check('P0 Direct message API is refused before inference', status == 409 and body.get('code') == 'qualification_required', (status, body))
    status, _ = alpha.call('POST', 'admin/state', {})
    check('Owner module routes are unreachable from a device', status == 403, status)

    # Photos over the tunnel.
    status, page = alpha.call('GET', 'photos/assets?size=100')
    ids = {p['id'] for p in page['items']}
    check('PH-01 Alpha lists own photos and videos only', env_data['photos_fixture']['see'] in ids and env_data['photos_fixture']['private_b'] not in ids)
    status, body = alpha.call('GET', f'photos/assets/{env_data["photos_fixture"]["see"]}/thumbnail', raw=True)
    check('PH-01 Thumbnail through the tunnel', status == 200 and len(body) > 100)
    status, _ = alpha.call('GET', f'photos/assets/{env_data["photos_fixture"]["private_b"]}/original', raw=True)
    check('PH-02 Foreign original denied over the tunnel', status in (403, 404), status)
    req = urllib.request.Request(alpha.base + f'/mutti/hub/v1/photos/assets/{env_data["photos_fixture"]["clip"]}/video', headers={'Range': 'bytes=0-1023'})
    with opener.open(req, timeout=60) as r:
        check('PH-01 Private video streams with Range', r.status == 206 and len(r.read()) == 1024)

    # Documents over the tunnel.
    status, docs = alpha.call('GET', 'documents?q=Rechnung')
    check('DO-01 Full-text search for own invoice', status == 200 and any(d['id'] == env_data['documents_fixture']['invoice'] for d in docs['items']), docs)
    status, docs = alpha.call('GET', 'documents?q=Arztrechnung')
    check('DO-01 Foreign document not found', docs['count'] == 0)
    status, body = alpha.call('GET', f'documents/{env_data["documents_fixture"]["invoice"]}/preview', raw=True)
    check('DO-01 Preview PDF through the tunnel', status == 200 and body[:4] == b'%PDF')

    # P1 shared content access over the tunnel, without any model.
    def search(device, **params):
        return device.call('GET', 'content/search?' + urllib.parse.urlencode(params))
    status, everything = search(alpha, size=50)
    raw_alpha = json.dumps(everything, ensure_ascii=False)
    kinds = {i['kind'] for i in everything['items']}
    check('CO-01 One search returns movies, photos, videos and documents', status == 200 and everything['completeness'] == 'complete' and {'movie', 'photo', 'video', 'document'} <= kinds, (status, kinds))
    check('CO-01 Areas are grouped media, photos, documents', [a['area'] for a in everything['areas']] == ['media', 'photos', 'documents'], everything['areas'])
    for secret in ('Privater Film B', env_data['photos_fixture']['private_b'], 'Arztrechnung', '999,99'):
        check(f'CO-02 Alpha sees nothing private of Beta ({secret[:12]})', secret not in raw_alpha)
    status, beta_all = search(beta, size=50)
    raw_beta = json.dumps(beta_all, ensure_ascii=False)
    check('CO-02 Beta sees its own private movie and photo', 'Privater Film B' in raw_beta and env_data['photos_fixture']['private_b'] in raw_beta)
    check('CO-02 Beta sees no Alpha photo or invoice', env_data['photos_fixture']['see'] not in raw_beta and 'RE-2026-0815' not in raw_beta)
    status, found = search(alpha, q='Rechnung', kinds='document')
    invoice = next((i for i in found['items'] if i['document']['id'] == env_data['documents_fixture']['invoice']), None)
    check('CO-03 Filtered full-text search finds the invoice with a revision', invoice is not None and invoice['ref']['revision'], found)
    cid_invoice = invoice['ref']['contentId']
    status, opened = alpha.call('GET', f'content/items/{cid_invoice}?revision={invoice["ref"]["revision"]}')
    check('CO-03 Opening re-authorizes and reports the revision as current', status == 200 and opened['revisionStatus'] == 'current', opened)
    status, body = alpha.call('GET', f'content/items/{cid_invoice}/thumbnail', raw=True)
    check('CO-03 Document preview through the content route', status == 200 and len(body) > 100)
    for path in (f'content/items/{cid_invoice}', f'content/items/{cid_invoice}/original'):
        status, _ = beta.call('GET', path, raw=True)
        check(f'CO-03 Beta cannot open Alpha content ({path.count("/")})', status == 404, status)
    status, page1 = search(alpha, size=1)
    status2, _ = beta.call('GET', 'content/search?cursor=' + urllib.parse.quote(page1['next']))
    status3, page2 = alpha.call('GET', 'content/search?cursor=' + urllib.parse.quote(page1['next']))
    check('CO-04 Cursor continues for its profile only', status2 == 409 and status3 == 200 and not ({i['ref']['contentId'] for i in page1['items']} & {i['ref']['contentId'] for i in page2['items']}), (status2, status3))
    status, jobs = alpha.call('GET', 'content/jobs')
    check('CO-05 Jobs list is available', status == 200 and isinstance(jobs['items'], list), jobs)
    photo_item = next(i for i in everything['items'] if i['kind'] == 'photo')

    if args.expect_qualified:
        # AI path AT-03 .. AT-11 with the real model.
        status, conv = alpha.call('POST', 'ai/conversations', {'title': ''})
        cid = conv['id']
        run, events, done = alpha.ask(cid, 'Suche alle ungesehenen Filme unter 100 Sekunden und sortiere nach Laufzeit.')
        report['at04'] = {'answer': done['message']['text'], 'sources': [s['title'] for s in done.get('sources', [])], 'events': len(events)}
        titles = [s['title'] for s in done.get('sources', [])]
        check('AT-04 Real Jellyfin tool call over profile rights returns the two expected movies', titles[:2] == ['Nordlicht', 'Sommer am See'] and 'Privater Film B' not in json.dumps(done), titles)
        check('AT-03 Streaming deltas and tool status before completion', any(e['type'] == 'delta' for e in events) and any(e['type'] == 'tool' for e in events))
        run2, _, done2 = alpha.ask(cid, 'Wie lange dauert der zweite Treffer?')
        report['at06'] = done2['message']['text']
        check('AT-06 Follow-up refers to the second hit (95 seconds)', '95' in done2['message']['text'], done2['message']['text'])
        invoice_pdf = subprocess.check_output([sys.executable, '-c', 'import importlib.util,sys;spec=importlib.util.spec_from_file_location("t",sys.argv[1]);t=importlib.util.module_from_spec(spec);spec.loader.exec_module(t);sys.stdout.buffer.write(t.pdf([["Stadtwerke Musterstadt","Rechnung RE-2026-0815","Rechnungsbetrag: 128,40 EUR","Faellig am 15.11.2026"]]))', str(repo / 'mutti/tests/module-testenv.py')])
        status, attachment = alpha.call('POST', f'ai/conversations/{cid}/attachments', invoice_pdf, {'Content-Type': 'application/pdf', 'X-Filename': 'rechnung.pdf'})
        check('AT-07 PDF attachment text extracted locally', status == 201 and attachment['extraction'] == 'ok', attachment)
        status, started = alpha.call('POST', f'ai/conversations/{cid}/messages', {'text': 'Wie hoch ist der Betrag dieser Rechnung und wann ist sie fällig?', 'idempotencyKey': 'att-1', 'attachments': [attachment['id']]})
        done3 = alpha.events(started['run']['id'], cid)[-1]['data']
        report['at07'] = done3['message']['text']
        check('AT-07 Amount and due date from the attachment with a valid source', '128,40' in done3['message']['text'] and '15.11.2026' in done3['message']['text'], done3['message']['text'])
        status, started = alpha.call('POST', f'ai/conversations/{cid}/messages', {'text': 'Erkläre ausführlich in mindestens zehn Absätzen, was eine Datensicherung ist.', 'idempotencyKey': 'long-1'})
        rid = started['run']['id']
        t_cancel = {}
        def cancel_on_delta(ev):
            if ev['type'] == 'delta' and 't' not in t_cancel:
                t_cancel['t'] = time.time()
                alpha.call('POST', f'ai/runs/{rid}/cancel', {})
            return False
        ev = alpha.events(rid, cid, stop=cancel_on_delta)
        cancelled_after = time.time() - t_cancel.get('t', time.time())
        report['cancelSeconds'] = round(cancelled_after, 2)
        check('AT-09 Cancel ends generation within the budget', ev[-1]['data']['state'] == 'cancelled' and cancelled_after < 5, (ev[-1]['data']['state'], cancelled_after))
        status, retry = alpha.call('POST', f'ai/runs/{rid}/retry', {'conversation': cid, 'idempotencyKey': 'retry-1'})
        status2, retry2 = alpha.call('POST', f'ai/runs/{rid}/retry', {'conversation': cid, 'idempotencyKey': 'retry-1'})
        check('AT-09 Retry starts exactly one new run', status == 202 and status2 == 200 and retry['run']['id'] == retry2['run']['id'])
        alpha.call('POST', f'ai/runs/{retry["run"]["id"]}/cancel', {})
        alpha.events(retry['run']['id'], cid)
        for path in (f'ai/conversations/{cid}', f'ai/sources/{cid}/Q1', f'ai/conversations/{cid}/attachments/{attachment["id"]}', f'ai/runs/{run}/events?conversation={cid}'):
            status, _ = beta.call('GET', path)
            check(f'AT-10 Beta cannot read Alpha data ({path.split("/")[1]})', status in (403, 404), status)
        status, lst = beta.call('GET', 'ai/conversations')
        check('AT-10 Beta sees no foreign conversations', status == 200 and lst['items'] == [])
        _, _, proposal_done = alpha.ask(cid, 'Markiere Nordlicht als Favorit.')
        props = proposal_done.get('proposals') or []
        report['at11'] = proposal_done['message']['text']
        check('AT-11 Favorite is only proposed', len(props) == 1 and props[0]['state'] == 'pending', proposal_done)
        fav = lambda: ok(jf('GET', f'/Items/{movie["Nordlicht"]}?userId={users["Alpha"]}', None, owner), 'item')['UserData']['IsFavorite']
        check('AT-11 Nothing changed before confirmation', fav() is False)
        status, p = beta.call('POST', f'ai/proposals/{props[0]["id"]}/confirm', {'conversation': cid})
        check('AT-16 Foreign confirmation rejected', status in (403, 404), status)
        status, p = alpha.call('POST', f'ai/proposals/{props[0]["id"]}/confirm', {'conversation': cid})
        check('AT-11 Confirmed action applied and verified by reading back', p['state'] == 'confirmed' and fav() is True, p)

    # Backups include module data (without models); the probe restores it.
    ok(jf('POST', '/Mutti/Maintenance/backup', {}, owner), 'backup')
    for _ in range(180):
        m = ok(jf('POST', '/Mutti/Maintenance/state', {}, owner), 'maintenance')
        if not m['busy'] and m['job'].get('kind') == 'backup':
            break
        time.sleep(2)
    check('Backup completes', m['job']['state'] == 'completed' and m['backups'], m['job'])
    backup_dir = Path(m['directory']) / m['backups'][0]['id']
    stored = json.loads((backup_dir / 'backup.json').read_text())
    hub_files = [k for k in stored['Hashes'] if k.startswith('hub/')]
    check('Backup contains module settings and content references but no model files',
          stored.get('Hub') is True and 'hub/hub.json' in hub_files and 'hub/content-ids.json' in hub_files and not any('models' in k for k in hub_files), hub_files)
    if args.expect_qualified:
        check('Backup contains conversations', any('/conversations/' in k for k in hub_files), hub_files)
    ok(jf('POST', '/Mutti/Maintenance/verify', {'ID': m['backups'][0]['id'], 'Username': 'Testbesitzer', 'Password': password, 'Confirm': True}, owner), 'verify')
    for _ in range(300):
        m = ok(jf('POST', '/Mutti/Maintenance/state', {}, owner), 'maintenance')
        if not m['busy'] and m['job'].get('kind') == 'verify':
            break
        time.sleep(2)
    check('Restore probe of the backup passes', m['job']['state'] == 'completed' and m['backups'][0].get('verifiedAt'), m['job'])
    # Rights changes apply immediately, also for modules and shared search.
    status, before = search(alpha, size=1)
    ok(hub_admin('admin/grants', {'module': 'photos', 'userId': users['Alpha'], 'allowed': False}), 'revoke grant')
    status, _ = alpha.call('GET', 'photos/assets')
    check('Revoked photo grant applies immediately over the tunnel', status == 403, status)
    status, after = search(alpha, size=50)
    photos_area = next(a for a in after['areas'] if a['area'] == 'photos')
    check('CO-06 Revoked area leaves shared search without photo hits', photos_area['state'] == 'not_available' and not any(i['area'] == 'photos' for i in after['items']), after['areas'])
    status, _ = alpha.call('GET', f'content/items/{photo_item["ref"]["contentId"]}')
    check('CO-06 Earlier photo reference no longer opens', status == 404, status)
    status, _ = alpha.call('GET', 'content/search?cursor=' + urllib.parse.quote(before['next']))
    check('CO-06 Cursor from before the rights change is refused', status == 409, status)
    # Module failure does not affect media and is not reported as zero results.
    project = json.loads(args.testenv.read_text())['project']
    subprocess.run(['docker', 'compose', '-p', project, '-f', str(args.testenv.parent / 'compose.json'), 'stop', 'paperless'], check=True, capture_output=True)
    try:
        time.sleep(25)
        caps = alpha.call('GET', f'capabilities?language={args.language}')[1]
        status, body = alpha.media(f'/Users/{users["Alpha"]}/Items?IncludeItemTypes=Movie&Recursive=true')
        check('Documents outage is reported while movies keep working', caps['modules']['documents']['state'] == 'unavailable' and status == 200, caps['modules']['documents'])
        status, partial = search(alpha, size=50)
        docs_area = next(a for a in partial['areas'] if a['area'] == 'documents')
        check('CO-07 Outage makes the search partial, movies still found', partial['completeness'] == 'partial' and docs_area['state'] == 'unavailable'
              and any(i['kind'] == 'movie' for i in partial['items']), partial['areas'])
    finally:
        subprocess.run(['docker', 'compose', '-p', project, '-f', str(args.testenv.parent / 'compose.json'), 'start', 'paperless'], check=True, capture_output=True)
    if not args.keep:
        # AT-12 revoking the device ends further access (and a running answer when AI is qualified).
        if args.expect_qualified:
            status, started = alpha.call('POST', f'ai/conversations/{cid}/messages', {'text': 'Erzähle ausführlich die Geschichte des Kinos.', 'idempotencyKey': 'revoke-1'})
            rid = started['run']['id']
            seen = {}
            def revoke_on_delta(ev):
                if ev['type'] == 'delta' and 't' not in seen:
                    seen['t'] = time.time()
                    ok(jf('POST', '/Mutti/Connect/revoke', {'pin': alpha.pin}, owner), 'revoke')
                return False
            try:
                alpha.events(rid, cid, stop=revoke_on_delta, timeout=30)
            except Exception:
                pass
        else:
            ok(jf('POST', '/Mutti/Connect/revoke', {'pin': alpha.pin}, owner), 'revoke')
        status, _ = alpha.call('GET', 'content/search', timeout=20)
        check('AT-12 Revoked device loses content access', status in (403, 502), status)
    else:
        # Native checks continue with the kept instance; restore Alpha's grant.
        ok(hub_admin('admin/grants', {'module': 'photos', 'userId': users['Alpha'], 'allowed': True}), 'restore grant')
    report['passed'] = True
except SetupDone:
    pass
except Exception as error:
    report['passed'] = False
    report['error'] = str(error)
    raise
finally:
    report['finished'] = time.time()
    (root / 'result.json').write_text(json.dumps(report, indent=2, ensure_ascii=False))
    if args.keep and report.get('passed'):
        print('Instance kept running. Owner credentials:', root / 'owner.json', flush=True)
        try:
            signal.pause()
        except KeyboardInterrupt:
            pass
    for p in reversed(processes):
        try:
            os.killpg(p.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    for p in processes:
        try:
            p.wait(timeout=30)
        except subprocess.TimeoutExpired:
            os.killpg(p.pid, signal.SIGKILL)
    for log in logs:
        log.close()
    print('Evidence:', root / 'result.json', flush=True)
