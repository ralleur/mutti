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
    with request(28594,'/') as response:assert b'Jellyfin' in response.read()
    with request(28597,'/System/Info/Public') as response:assert not json.load(response)['StartupWizardCompleted']
    for headers in ({'Host':'attacker.example'},{'Origin':'https://attacker.example'}):
        try:request(28597,'/System/Info/Public',headers=headers)
        except urllib.error.HTTPError as e:assert e.code==403
        else:raise AssertionError('foreign host/origin accepted')
    try: request(28595,'/')
    except (OSError,urllib.error.URLError):pass
    else:raise AssertionError('pairing started before setup')
    with request(28594,'/api/session') as response:csrf=json.load(response)['csrf']
    with request(28594,'/api/new',{}, {'Content-Type':'application/json','X-Mutti-CSRF':csrf,'Origin':'http://127.0.0.1:28594'}) as response:assert json.load(response)['target']=='http://127.0.0.1:28597/web/'
    with request(28597,'/Startup/User') as response:json.load(response)
    bindings=json.loads(subprocess.check_output(['docker','inspect',name]))[0]['HostConfig']['PortBindings']
    assert all(binding['HostIp']=='127.0.0.1' for port in bindings.values() for binding in port)
    print('PASS: actual Docker entrypoint, onboarding choice, browser access, Host/Origin rejection, loopback-only published ports, pairing gated until setup')
finally:
    with (work/'container.log').open('w') as log:subprocess.run(['docker','logs',name],stdout=log,stderr=log)
    subprocess.run(['docker','rm','-f',name],stdout=subprocess.DEVNULL,check=True)
print('Synthetic package artifacts:',work)
