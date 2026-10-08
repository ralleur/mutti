#!/usr/bin/env python3
"""Verify the actual Docker entrypoint and its published loopback boundary.
Only new synthetic directories and this script's uniquely named container are used.
"""
from pathlib import Path
import json, os, socket, subprocess, tempfile, time, urllib.request, urllib.error
root=Path(__file__).resolve().parents[2]
work=Path(tempfile.mkdtemp(prefix='mutti-import-package-'))
config=work/'config';config.mkdir(mode=0o700)
for port in (28594,28595,28597):
    with socket.socket() as probe: probe.bind(('127.0.0.1',port))
name='mutti-import-package-'+work.name.rsplit('-',1)[-1]
args=['docker','run','-d','--name',name,'--user',f'{os.getuid()}:{os.getgid()}','--read-only','--cap-drop=ALL','--security-opt=no-new-privileges:true','--tmpfs','/tmp:rw,noexec,nosuid,size=256m','-p','127.0.0.1:28594:18594','-p','127.0.0.1:28595:18595','-p','127.0.0.1:28597:8096','-v',f'{config}:/config','mutti:import-preview','--origin','http://127.0.0.1:28594','--target-origin','http://127.0.0.1:28597','--connect-origin','http://127.0.0.1:28595']
subprocess.run(args,check=True,stdout=subprocess.DEVNULL)
def request(port,path,body=None,headers=None):
    req=urllib.request.Request(f'http://127.0.0.1:{port}'+path,data=None if body is None else json.dumps(body).encode(),headers=headers or {})
    return urllib.request.urlopen(req,timeout=3)
try:
    for _ in range(90):
        try:
            with request(28594,'/api/state') as response: state=json.load(response)
            if state['ready']:break
        except (OSError,urllib.error.URLError):pass
        time.sleep(1)
    else:raise AssertionError('manager did not become ready')
    assert not state['setupComplete'] and not state['newSetup']
    assert (config/'data/plugins/Intro Skipper_12.0.4.0/IntroSkipper.dll').is_file()
    with request(28594,'/') as response:assert b'Jellyfin' in response.read()
    for asset in ['symbol.svg','mark-light.svg','wordmark-light.svg','sora.woff2','sora-semibold.woff2','sora-bold.woff2']:
        with request(28594,'/'+asset) as response:
            expected_type='image/svg+xml' if asset.endswith('.svg') else 'font/woff2'
            assert response.headers['Content-Type'].startswith(expected_type)
            assert response.read()==(root/'mutti/migrate/web'/asset).read_bytes()
    with request(28594,'/kurt/manifest.json') as response:
        kurt=json.load(response)
    assert all(clip in kurt['clips'] for clip in ('idle-sleep','wake-seat','rise-seat','scoot-floor','walk-floor','run-floor','eat-floor','vomit-floor','poop-floor','turn-floor'))
    for image in kurt['pages']+['rest.png','poop.png','vomit.png','poop-drop.png','vomit-drop.png']:
        with request(28594,'/kurt/'+image) as response:
            assert response.headers['Content-Type'].startswith('image/')
            assert len(response.read(32))==32
    with request(28594,'/kurt.js') as response:assert b'MuttiKurt' in response.read()
    with request(28597,'/System/Info/Public') as response:assert not json.load(response)['StartupWizardCompleted']
    for headers in ({'Host':'attacker.example'},{'Origin':'https://attacker.example'}):
        try:request(28597,'/System/Info/Public',headers=headers)
        except urllib.error.HTTPError as e:assert e.code==403
        else:raise AssertionError('foreign host/origin accepted')
    try: request(28595,'/')
    except (OSError,urllib.error.URLError):pass
    else:raise AssertionError('pairing started before setup')
    with request(28594,'/api/session') as response:
        session=json.load(response);csrf=session['csrf'];assert session['nativeOwner'] is False
    with request(28594,'/api/session',headers={'X-Mutti-Native-Owner':csrf}) as response:assert json.load(response)['nativeOwner'] is False
    with request(28594,'/api/new',{}, {'Content-Type':'application/json','X-Mutti-CSRF':csrf,'Origin':'http://127.0.0.1:28594'}) as response:assert json.load(response)['target']=='http://127.0.0.1:28597/web/'
    with request(28597,'/Startup/User') as response:json.load(response)
    bindings=json.loads(subprocess.check_output(['docker','inspect',name]))[0]['HostConfig']['PortBindings']
    assert all(binding['HostIp']=='127.0.0.1' for port in bindings.values() for binding in port)
    print('PASS: actual Docker entrypoint, bundled branding/fonts and Kurt clips/images, onboarding choice, browser access, Host/Origin rejection, loopback-only published ports, pairing gated until setup')
finally:
    with (work/'container.log').open('w') as log:subprocess.run(['docker','logs',name],stdout=log,stderr=log)
    subprocess.run(['docker','rm','-f',name],stdout=subprocess.DEVNULL,check=True)
print('Synthetic package artifacts:',work)
