#!/usr/bin/env python3
"""Exercise the packaged management API using only a fresh synthetic container.
--keep retains this isolated fixture for browser QA; remove the printed container afterwards.
"""
import argparse
import json
import http.client
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('--keep', action='store_true')
parser.add_argument('--image', default='mutti:import-preview')
parser.add_argument('--maintenance', action='store_true')
args = parser.parse_args()
work = Path(tempfile.mkdtemp(prefix='mutti-management-package-'))
config = work / 'config'
config.mkdir(mode=0o700)
(config / 'media').mkdir()
ports = (29594, 29595, 29597)
for port in ports:
    with socket.socket() as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        probe.bind(('127.0.0.1', port))
name = work.name
command = ['docker', 'run', '-d', '--name', name, '--user', f'{os.getuid()}:{os.getgid()}',
           '--read-only', '--cap-drop=ALL', '--security-opt=no-new-privileges:true',
           '--tmpfs', '/tmp:rw,noexec,nosuid,size=256m']
for host, container in zip(ports, (18594, 18595, 8096)):
    command += ['-p', f'127.0.0.1:{host}:{container}']
command += ['-v', f'{config}:/config', args.image, '--origin',
            'http://127.0.0.1:29594', '--target-origin', 'http://127.0.0.1:29597',
            '--connect-origin', 'http://127.0.0.1:29595']
subprocess.run(command, check=True, stdout=subprocess.DEVNULL)
passed = False

def request(path, data=None, token=None, port=29597, headers=None, method=None):
    auth = 'MediaBrowser Client="Mutti Management Test", Device="Synthetic", DeviceId="management-test", Version="0.1"'
    if token:
        auth += f', Token="{token}"'
    final_headers = {'Authorization': auth, 'Content-Type': 'application/json'}
    final_headers.update(headers or {})
    req = urllib.request.Request(f'http://127.0.0.1:{port}{path}',
                                 data=json.dumps(data).encode() if data is not None else None,
                                 headers=final_headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=20) as response:
            return response.status, response.headers, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.headers, error.read()

def api(path, data=None, token=None, **kwargs):
    status, _, body = request(path, data, token, **kwargs)
    assert status in (200, 204), f'{path.split("?")[0]}: unexpected HTTP {status}'
    return json.loads(body) if body else None

def check(label, condition):
    assert condition, label
    print('PASS:', label, flush=True)

try:
    for _ in range(90):
        try:
            state = api('/api/state', port=29594)
            if state['ready']:
                break
        except (OSError, AssertionError):
            pass
        time.sleep(1)
    else:
        raise AssertionError('Fresh package did not become ready')
    assert not state['setupComplete'] and not state['newSetup']
    assert not api('/System/Info/Public')['StartupWizardCompleted']
    session = api('/api/session', port=29594)
    api('/api/new', {}, port=29594, headers={'X-Mutti-CSRF': session['csrf'], 'Origin': 'http://127.0.0.1:29594'})
    api('/Startup/User')
    api('/Startup/Configuration', {'ServerName': 'Mutti Testserver', 'UICulture': 'de',
                                 'MetadataCountryCode': 'DE', 'PreferredMetadataLanguage': 'de'})
    password = secrets.token_urlsafe(24)
    api('/Startup/User', {'Name': 'Testbesitzer', 'Password': password})
    api('/Startup/RemoteAccess', {'EnableRemoteAccess': False})
    owner = api('/Users/AuthenticateByName', {'Username': 'Testbesitzer', 'Pw': password})
    token = owner['AccessToken']
    api('/Startup/Complete', {}, token)
    credentials = work / 'browser-fixture.json'
    credentials.write_text(json.dumps({'username': 'Testbesitzer', 'password': password, 'url': 'http://127.0.0.1:29597/web/#/mutti'}))
    credentials.chmod(0o600)
    anonymous = (request('/Mutti/Management')[0], request('/Mutti/Connect/state', {})[0])
    check(f'Anonymous management and pairing are denied ({anonymous})', anonymous == (401, 401))
    management = api('/Mutti/Management', token=token)
    check('Package provides its exact onboarding origin and Connect capability',
          management == {'onboardingUrl': 'http://127.0.0.1:29594/#import', 'connectAvailable': True})
    for _ in range(30):
        status, _, body = request('/Mutti/Connect/state', {}, token)
        if status == 200:
            connect = json.loads(body)
            break
        time.sleep(1)
    else:
        raise AssertionError(f'Packaged Connect bridge did not start: HTTP {status}')
    check('Authenticated owner reuses existing login for Connect', connect['devices'] == [] and connect['pending'] == [] and connect['relay'] is False)
    profile = api('/Mutti/Connect/profile', {'Name': 'Familie'}, token)
    check('Creating a playback profile grants no administrator rights', not profile['Policy']['IsAdministrator'])
    check('New profile appears in live Connect state', profile['Id'] in [p['Id'] for p in api('/Mutti/Connect/state', {}, token)['profiles']])
    invite = api('/Mutti/Connect/invite', {}, token)
    check('Pairing has a real bounded Unix expiry', time.time() < invite['expires'] <= time.time() + 660)
    status, headers, png = request('/Mutti/Connect/qr', {'url': invite['url']}, token)
    check('Authenticated QR response is a PNG without caching', status == 200 and png.startswith(b'\x89PNG\r\n\x1a\n') and headers['Cache-Control'] == 'no-store')
    check('Bridge preserves failure status and disallows arbitrary actions',
          request('/Mutti/Connect/qr', {'url': 'invalid'}, token)[0] == 400 and
          request('/Mutti/Connect/login', {}, token)[0] == 404)
    oversized = request('/Mutti/Connect/qr', {'url': invite['url'], 'padding': 'x' * 17000}, token)[0]
    check(f'Bridge rejects oversized otherwise-valid QR payload (HTTP {oversized})', oversized == 413)
    chunked = http.client.HTTPConnection('127.0.0.1', 29597, timeout=15)
    chunked.request('POST', '/Mutti/Connect/qr',
                    body=iter([json.dumps({'url': invite['url'], 'padding': 'x' * 17000}).encode()]),
                    headers={'Authorization': f'MediaBrowser Client="Mutti Management Test", Device="Synthetic", DeviceId="management-test", Version="0.1", Token="{token}"', 'Content-Type': 'application/json'},
                    encode_chunked=True)
    response = chunked.getresponse()
    check('Chunked oversized JSON is also rejected', response.status == 413)
    response.read()
    chunked.close()
    for header in ({'Origin': 'https://attacker.example'}, {'Host': 'attacker.example'}):
        check('Foreign browser origin/host rejected', request('/Mutti/Management', token=token, headers=header)[0] == 403)
    viewer_password = secrets.token_urlsafe(24)
    api('/Users/New', {'Name': 'Viewer', 'Password': viewer_password}, token)
    viewer = api('/Users/AuthenticateByName', {'Username': 'Viewer', 'Pw': viewer_password})['AccessToken']
    check('Non-admin cannot read management or mutate pairing',
          request('/Mutti/Management', token=viewer)[0] == 403 and
          request('/Mutti/Connect/profile', {'Name': 'Forbidden'}, viewer)[0] == 403)
    assert api('/Library/VirtualFolders', token=token) == []
    query = urllib.parse.urlencode({'name': 'Testfilme', 'collectionType': 'movies', 'paths': '/config/media', 'refreshLibrary': 'false'})
    api('/Library/VirtualFolders?' + query, {'LibraryOptions': {'EnableRealtimeMonitor': False, 'EnableInternetProviders': False}}, token)
    libraries = api('/Library/VirtualFolders', token=token)
    check('Real add-library API persists synthetic folder', len(libraries) == 1 and libraries[0]['Locations'] == ['/config/media'])
    counts = api('/Items?ParentId=' + libraries[0]['ItemId'] + '&Recursive=true&IsFolder=false&Limit=0&EnableTotalRecordCount=true', token=token)
    check('Actual empty library has zero records', counts['TotalRecordCount'] == 0)
    viewer_info = api('/Users/Me', token=viewer)
    policy_before = viewer_info['Policy']
    restricted = dict(policy_before, EnableAllFolders=False, EnabledFolders=[], EnableMediaPlayback=False)
    api('/Users/' + viewer_info['Id'] + '/Policy', restricted, token)
    rights = api('/Users/Me', token=viewer)['Policy']
    check('Restricted profile policy is enforced and unrelated rights retained',
          rights['EnableAllFolders'] is False and rights['EnableMediaPlayback'] is False and
          all(rights[k] == v for k,v in policy_before.items() if k not in ('EnableAllFolders', 'EnabledFolders', 'EnableMediaPlayback')))
    check('No libraries leak to profile without folder access', not api('/UserViews', token=viewer)['Items'])
    allowed = dict(restricted, EnabledFolders=[libraries[0]['ItemId']], EnableMediaPlayback=True)
    api('/Users/' + viewer_info['Id'] + '/Policy', allowed, token)
    check('Explicit library grant works through existing user API', any(v['Id'] == libraries[0]['ItemId'] for v in api('/UserViews', token=viewer)['Items']))
    storage = api('/System/Info/Storage', token=token)
    check('Storage comes from packaged server', storage['ProgramDataFolder']['FreeSpace'] > 0)
    original = api('/System/Configuration', token=token)
    changed = dict(original, ServerName='Mutti Testserver geprüft')
    api('/System/Configuration', changed, token)
    actual = api('/System/Configuration', token=token)
    check('Server rename preserves other configuration values', all(actual[key] == value for key, value in original.items() if key != 'ServerName') and actual['ServerName'] == changed['ServerName'])
    api('/Library/Refresh', {}, token)
    check('Scheduled library work stays visible', any(task['Key'] == 'RefreshLibrary' for task in api('/ScheduledTasks', token=token)))
    check('Packaged web retains Mutti brand', b'<title>Mutti</title>' in request('/web/index.html')[2])
    if args.maintenance:
        check('Maintenance requires administrator rights',
              request('/Mutti/Maintenance/state', {})[0] == 401 and
              request('/Mutti/Maintenance/backup', {}, viewer)[0] == 403)
        check('No arbitrary manager actions are forwarded', request('/Mutti/Maintenance/import', {}, token)[0] == 404)
        before = api('/api/state', port=29594)['active']
        def wait_job(expected, auth_token):
            for _ in range(300):
                state = api('/Mutti/Maintenance/state', {}, auth_token)
                if state['job']['state'] != 'running':
                    check('Maintenance job ' + expected, state['job']['state'] == expected)
                    return state
                time.sleep(1)
            raise AssertionError('Maintenance job timed out')
        check('Backup accepted asynchronously', request('/Mutti/Maintenance/backup', {}, token)[0] == 202)
        backup_state = wait_job('completed', token)
        backup = backup_state['backups'][0]
        check('New backup has no invented restore proof', backup['bytes'] > 0 and not backup.get('verifiedAt'))
        invalid = {'ID': backup['id'], 'Username': 'Testbesitzer', 'Password': 'incorrect', 'Confirm': True}
        check('Probe accepted', request('/Mutti/Maintenance/verify', invalid, token)[0] == 202)
        wait_job('failed', token)
        check('Failed probe retains current instance', api('/api/state', port=29594)['active'] == before)
        restore = dict(invalid, Password=password)
        check('Valid probe accepted', request('/Mutti/Maintenance/verify', restore, token)[0] == 202)
        checked = wait_job('completed', token)
        check('Probe records actual verification and keeps active data', checked['backups'][0].get('verifiedAt') and api('/api/state', port=29594)['active'] == before)
        # Make a real reversible change after the backup, then verify its rollback.
        rename = api('/System/Configuration', token=token)
        rename['ServerName'] = 'Changed after backup'
        api('/System/Configuration', rename, token)
        check('Restore accepted', request('/Mutti/Maintenance/restore', restore, token)[0] == 202)
        for _ in range(300):
            s = api('/api/state', port=29594)
            if s['active'] != before and s['ready']:
                break
            time.sleep(1)
        else:
            raise AssertionError('Restore did not activate')
        check('Previous session is rejected after restore', request('/Mutti/Maintenance/state', {}, token)[0] == 401)
        token = api('/Users/AuthenticateByName', {'Username': 'Testbesitzer', 'Pw': password})['AccessToken']
        wait_job('completed', token)
        check('Actual restored configuration matches backup', api('/System/Configuration', token=token)['ServerName'] == changed['ServerName'])
        check('Previous instance retained', (config / 'data').is_dir())
        # Kill only Jellyfin in this uniquely named synthetic container.
        subprocess.run(['docker', 'exec', name, 'sh', '-c',
                        'for p in /proc/[0-9]*; do if [ "$(readlink "$p/exe")" = /jellyfin/jellyfin ]; then kill -KILL "${p##*/}"; fi; done'], check=True)
        time.sleep(2)
        for _ in range(90):
            try:
                if api('/api/state', port=29594)['ready'] and api('/System/Info/Public')['StartupWizardCompleted']:
                    break
            except (OSError, AssertionError):
                pass
            time.sleep(1)
        else:
            raise AssertionError('Jellyfin did not recover after crash')
        check('Automatic restart preserves library and owner session', len(api('/Library/VirtualFolders', token=token)) == 1)
    passed = True
finally:
    with (work / 'container.log').open('w') as log:
        subprocess.run(['docker', 'logs', name], stdout=log, stderr=log)
    if not (args.keep and passed):
        subprocess.run(['docker', 'rm', '-f', name], check=True, stdout=subprocess.DEVNULL)
    else:
        print('Browser QA container:', name)
        print('Private fixture:', credentials)
print('Synthetic package artifacts:', work)
