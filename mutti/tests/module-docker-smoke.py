#!/usr/bin/env python3
"""Module test of the Docker package with a fresh synthetic container.

The container runs the unchanged package entrypoint (manager, Jellyfin,
Connect, module service). Photos/documents use the isolated test environment
via host.docker.internal; the optional AI part uses an owner-provided engine on
the host (Docker packages do not bundle an engine). A paired test device runs
inside the container so the encrypted tunnel uses loopback ICE. Only this
uniquely named container is removed at the end.
"""
import argparse
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--image', required=True)
parser.add_argument('--testenv', type=Path, required=True)
parser.add_argument('--engine', help='host engine URL as seen from the container, e.g. http://host.docker.internal:31434')
parser.add_argument('--model')
args = parser.parse_args()
repo = Path(__file__).resolve().parents[2]
env_data = json.loads(args.testenv.read_text())
work = Path(tempfile.mkdtemp(prefix='mutti-module-docker-'))
for d in ('config', 'cache', 'media', 'runner'):
    (work / d).mkdir(mode=0o700)
ports = (29694, 29695, 29697)
for port in ports:
    with socket.socket() as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        probe.bind(('127.0.0.1', port))
subprocess.run(['go', 'build', '-o', str(work / 'runner/mutti-testdevice'), './cmd/mutti-testdevice'], cwd=repo / 'mutti/connect',
               env=dict(os.environ, GOOS='linux', GOARCH='arm64', CGO_ENABLED='0'), check=True)
ffmpeg = repo / 'build/macos/osx-arm64/Mutti.app/Contents/Resources/ffmpeg/ffmpeg'
for name, seconds in (('Nordlicht (2024)', 84), ('Sommer am See (2023)', 95), ('Lange Reise (2022)', 125)):
    subprocess.run([str(ffmpeg), '-v', 'error', '-f', 'lavfi', '-i', 'testsrc2=size=320x180:rate=10', '-t', str(seconds), '-c:v', 'libx264',
                    '-preset', 'ultrafast', '-pix_fmt', 'yuv420p', '-an', str(work / 'media' / f'{name}.mp4')], check=True)
container = 'mutti-module-smoke-' + secrets.token_hex(4)
command = ['docker', 'run', '-d', '--name', container, '--user', f'{os.getuid()}:{os.getgid()}', '--read-only', '--cap-drop=ALL',
           '--security-opt=no-new-privileges:true', '--tmpfs', '/tmp:rw,noexec,nosuid,size=256m',
           '-e', 'MUTTI_LAN_ORIGIN=http://127.0.0.1:18599', '-e', 'MUTTI_ICE_INTERFACES=lo',
           '-v', f'{work}/config:/config', '-v', f'{work}/cache:/cache', '-v', f'{work}/media:/media:ro', '-v', f'{work}/runner:/smoke:ro']
for host, inner in zip(ports, (18594, 18595, 8096)):
    command += ['-p', f'127.0.0.1:{host}:{inner}']
command += [args.image, '--origin', 'http://127.0.0.1:29694', '--target-origin', 'http://127.0.0.1:29697', '--connect-origin', 'http://127.0.0.1:29695']
subprocess.run(command, check=True, stdout=subprocess.DEVNULL)
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
report = {'image': args.image, 'checks': []}


def check(label, condition, detail=None):
    if not condition:
        raise AssertionError(label + (f' ({detail})' if detail is not None else ''))
    report['checks'].append(label)
    print('PASS:', label, flush=True)


def jf(method, path, body=None, token=None):
    auth = 'MediaBrowser Client="Module Docker", Device="Synthetic", DeviceId="module-docker", Version="0.1"' + (f', Token="{token}"' if token else '')
    req = urllib.request.Request(f'http://127.0.0.1:29697{path}', data=json.dumps(body).encode() if body is not None else None,
                                 headers={'Authorization': auth, 'Content-Type': 'application/json'}, method=method)
    try:
        with opener.open(req, timeout=90) as r:
            data = r.read()
            return r.status, json.loads(data) if data else None
    except urllib.error.HTTPError as e:
        data = e.read()
        try:
            return e.code, json.loads(data)
        except ValueError:
            return e.code, data


def ok(result, label):
    if result[0] not in (200, 201, 202, 204):
        raise AssertionError(f'{label}: {result}')
    return result[1]


def device_call(method, path, body=None):
    cmd = ['docker', 'exec', container, 'curl', '-s', '-m', '240', '-X', method, '-w', '\\n%{http_code}', gateway + '/mutti/hub/v1/' + path]
    if body is not None:
        cmd += ['-H', 'Content-Type: application/json', '-d', json.dumps(body)]
    out = subprocess.run(cmd, capture_output=True, text=True).stdout
    payload, _, code = out.rpartition('\n')
    try:
        return int(code), json.loads(payload) if payload else None
    except ValueError:
        return int(code or 0), payload


try:
    for _ in range(120):
        try:
            if jf('GET', '/Startup/User')[0] == 200:
                break
        except OSError:
            pass
        time.sleep(1)
    password = secrets.token_urlsafe(24)
    ok(jf('POST', '/Startup/Configuration', {'ServerName': 'Mutti Docker Module', 'UICulture': 'de', 'MetadataCountryCode': 'DE', 'PreferredMetadataLanguage': 'de'}), 'config')
    ok(jf('POST', '/Startup/User', {'Name': 'Testbesitzer', 'Password': password}), 'user')
    ok(jf('POST', '/Startup/RemoteAccess', {'EnableRemoteAccess': False}), 'remote')
    owner = ok(jf('POST', '/Users/AuthenticateByName', {'Username': 'Testbesitzer', 'Pw': password}), 'login')['AccessToken']
    ok(jf('POST', '/Startup/Complete', {}, owner), 'complete')
    alpha = ok(jf('POST', '/Users/New', {'Name': 'Alpha', 'Password': secrets.token_urlsafe(24)}, owner), 'profile')['Id']
    ok(jf('POST', '/Library/VirtualFolders?name=Filme&collectionType=movies&paths=%2Fmedia&refreshLibrary=true',
          {'LibraryOptions': {'TypeOptions': [{'Type': 'Movie', 'MetadataFetchers': [], 'ImageFetchers': []}]}}, owner), 'library')
    for _ in range(90):
        status, state = jf('POST', '/Mutti/Hub/admin/state', {}, owner)
        if status == 200:
            break
        time.sleep(1)
    check('Docker package starts the module service after setup', status == 200, state)
    check('Docker reports no bundled AI engine', state['modules']['ai']['ai']['engine']['installed'] is False, state['modules']['ai']['ai']['engine'])
    photos_url = env_data['photos_url'].replace('127.0.0.1', 'host.docker.internal')
    docs_url = env_data['documents_url'].replace('127.0.0.1', 'host.docker.internal')
    status, body = jf('POST', '/Mutti/Hub/admin/service/photos', {'url': 'http://1.1.1.1:2283'}, owner)
    check('Public service address rejected inside Docker', status == 400, body)
    ok(jf('POST', '/Mutti/Hub/admin/service/photos', {'url': photos_url}, owner), 'photos service')
    ok(jf('POST', '/Mutti/Hub/admin/service/documents', {'url': docs_url}, owner), 'documents service')
    a = env_data['accounts']
    ok(jf('POST', '/Mutti/Hub/admin/link/photos', {'userId': alpha, 'account': a['photos']['alpha']['email'], 'password': a['photos']['alpha']['password']}, owner), 'link photos')
    ok(jf('POST', '/Mutti/Hub/admin/link/documents', {'userId': alpha, 'account': a['documents']['alpha']['username'], 'password': a['documents']['alpha']['password']}, owner), 'link docs')
    for module in ('photos', 'documents', 'ai'):
        ok(jf('POST', '/Mutti/Hub/admin/grants', {'module': module, 'userId': alpha, 'allowed': True}, owner), 'grant')
    ok(jf('POST', '/Mutti/Hub/admin/enable/photos', {'enabled': True}, owner), 'enable')
    ok(jf('POST', '/Mutti/Hub/admin/enable/documents', {'enabled': True}, owner), 'enable')
    if args.engine:
        ok(jf('POST', '/Mutti/Hub/admin/ai/engine', {'mode': 'external', 'url': args.engine}, owner), 'engine')
        ok(jf('POST', '/Mutti/Hub/admin/ai/models/select', {'model': args.model}, owner), 'select')
        ok(jf('POST', '/Mutti/Hub/admin/enable/ai', {'enabled': True}, owner), 'enable ai')
    invite = ok(jf('POST', '/Mutti/Connect/invite', {}, owner), 'invite')['url']
    device = subprocess.Popen(['docker', 'exec', '-e', 'MUTTI_ICE_INTERFACES=lo', container, '/smoke/mutti-testdevice', '--invite', invite,
                               '--name', 'Docker-Testgeraet', '--credentials', '/tmp/device.json'], stdout=subprocess.PIPE, text=True)
    assert json.loads(device.stdout.readline())['state'] == 'pairing'
    for _ in range(60):
        pending = ok(jf('POST', '/Mutti/Connect/state', {}, owner), 'state')['pending']
        if pending:
            ok(jf('POST', '/Mutti/Connect/approve', {'pin': pending[0]['pin'], 'userId': alpha}, owner), 'approve')
            break
        time.sleep(0.5)
    ready = json.loads(device.stdout.readline())
    gateway = ready['gateway']
    check('Device paired inside the container over the encrypted tunnel', ready['state'] == 'ready')
    for _ in range(30):
        status, caps = device_call('GET', 'capabilities')
        if status == 200 and caps['modules']['photos']['state'] == 'ready' and caps['modules']['documents']['state'] == 'ready':
            break
        time.sleep(2)
    check('Device sees photos and documents as ready', caps['modules']['photos']['state'] == 'ready' and caps['modules']['documents']['state'] == 'ready', caps)
    status, page = device_call('GET', 'photos/assets?size=50')
    check('Photos list through Docker package', status == 200 and any(p['id'] == env_data['photos_fixture']['see'] for p in page['items']))
    status, docs = device_call('GET', 'documents?q=Rechnung')
    check('Document search through Docker package', status == 200 and docs['count'] >= 1)
    if args.engine:
        status, conv = device_call('POST', 'ai/conversations', {'title': ''})
        status, started = device_call('POST', f'ai/conversations/{conv["id"]}/messages', {'text': 'Suche alle Filme unter 100 Sekunden.', 'idempotencyKey': 'docker-1'})
        out = subprocess.run(['docker', 'exec', container, 'curl', '-s', '-N', '-m', '240', f'{gateway}/mutti/hub/v1/ai/runs/{started["run"]["id"]}/events?conversation={conv["id"]}'],
                             capture_output=True, text=True).stdout
        done = [json.loads(l[6:]) for l in out.splitlines() if l.startswith('data: ')][-1]
        titles = [s['title'] for s in done.get('sources') or []]
        report['ai'] = {'answer': done['message']['text'], 'sources': titles}
        check('Docker AI answers with the external private engine and real library rights', done['state'] == 'completed' and 'Nordlicht' in titles and 'Lange Reise' not in titles, done['message']['text'])
    ok(jf('POST', '/Mutti/Connect/revoke', {'pin': ready['pin']}, owner), 'revoke')
    status, _ = device_call('GET', 'capabilities')
    check('Revoked device loses module access in Docker', status in (403, 502), status)
    report['passed'] = True
finally:
    with (work / 'container.log').open('w') as log:
        subprocess.run(['docker', 'logs', container], stdout=log, stderr=log)
    subprocess.run(['docker', 'rm', '-f', container], stdout=subprocess.DEVNULL)
    (work / 'result.json').write_text(json.dumps(report, indent=2, ensure_ascii=False))
    print('Evidence:', work / 'result.json')
