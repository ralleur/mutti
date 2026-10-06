#!/usr/bin/env python3
"""Keep a new synthetic Mac server alive for testing the existing kurtz app.
Ctrl-C stops only this fixture's manager/broker. Its private data is retained.
"""
import argparse
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import time
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--root', type=Path, required=True)
args = parser.parse_args()
repo = Path(__file__).resolve().parents[2]
root = args.root.resolve()
if root.exists(): raise SystemExit('Use a new fixture directory; no existing data is accepted')
root.mkdir(parents=True, mode=0o700)
media = root / 'media'; media.mkdir()
for port in (30594,30595,30596,30600):
    with socket.socket() as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        probe.bind(('127.0.0.1',port))
ffmpeg = repo / 'build/ffmpeg/osx-arm64/ffmpeg'
subprocess.run([str(ffmpeg),'-v','error','-f','lavfi','-i','testsrc2=size=640x360:rate=24','-t','30',
                '-c:v','libx264','-pix_fmt','yuv420p','-an',str(media / 'Mutti Testfilm (2026).mp4')],check=True)
processes=[]; logs=[]
def start(binary, params, env=None):
    log=(root / (binary.name+'.log')).open('w');logs.append(log)
    p=subprocess.Popen([str(binary)]+params,env=env,stdout=log,stderr=log);processes.append(p)
    return p
def api(path, body=None, token=''):
    auth='MediaBrowser Client="Mutti Fixture", Device="Test", DeviceId="native-fixture", Version="0.1"'
    if token: auth+=', Token="'+token+'"'
    req=urllib.request.Request('http://127.0.0.1:30596'+path,data=json.dumps(body).encode() if body is not None else None,
                                headers={'Authorization':auth,'Content-Type':'application/json'})
    with urllib.request.urlopen(req,timeout=15) as response:
        data=response.read();return json.loads(data) if data else None
try:
    start(repo/'build/connect/osx-arm64/mutti-connect',['--mode','broker','--listen','127.0.0.1:30600','--stun-listen','127.0.0.1:0'])
    env=dict(os.environ,MUTTI_SIGNAL_URL='http://127.0.0.1:30600')
    start(repo/'build/migrate/osx-arm64/mutti-migrate',['--root',str(root/'server'),'--server',str(repo/'build/server/osx-arm64/jellyfin'),
        '--web',str(repo.parent/'mutti-web/dist'),'--ffmpeg',str(ffmpeg),'--intro-skipper',str(repo/'build/intro-skipper'),
        '--connect',str(repo/'build/connect/osx-arm64/mutti-connect'),'--listen','127.0.0.1:30594','--origin','http://127.0.0.1:30594',
        '--backend','http://127.0.0.1:30596','--target-origin','http://127.0.0.1:30596','--connect-listen','127.0.0.1:30595',
        '--connect-origin','http://127.0.0.1:30595'],env)
    for _ in range(90):
        try: api('/Startup/User');break
        except OSError: time.sleep(1)
    else: raise RuntimeError('Fixture did not start')
    password=secrets.token_urlsafe(24)
    api('/Startup/Configuration',{'ServerName':'Mutti Native Test','UICulture':'de','MetadataCountryCode':'DE','PreferredMetadataLanguage':'de'})
    api('/Startup/User',{'Name':'Testbesitzer','Password':password})
    api('/Startup/RemoteAccess',{'EnableRemoteAccess':False})
    token=api('/Users/AuthenticateByName',{'Username':'Testbesitzer','Pw':password})['AccessToken']
    api('/Startup/Complete',{},token)
    viewer=api('/Users/New',{'Name':'Testprofil','Password':secrets.token_urlsafe(24)},token)
    from urllib.parse import urlencode
    api('/Library/VirtualFolders?'+urlencode({'name':'Testfilme','collectionType':'movies','paths':str(media),'refreshLibrary':'true'}),
        {'LibraryOptions':{'EnableRealtimeMonitor':False,'EnableInternetProviders':False,'TypeOptions':[{'Type':'Movie','MetadataFetchers':[],'ImageFetchers':[]}]}},token)
    for _ in range(90):
        if api('/Items?Recursive=true&IncludeItemTypes=Movie',token=token)['Items']:break
        time.sleep(1)
    for _ in range(30):
        try:
            invitation=api('/Mutti/Connect/invite',{},token);break
        except OSError:time.sleep(1)
    else: raise RuntimeError('Fixture pairing did not start')
    credentials=root/'private.json'
    credentials.write_text(json.dumps({'username':'Testbesitzer','password':password,'token':token,'viewerId':viewer['Id'],
                                        'invitation':invitation['url'],'url':'http://127.0.0.1:30596/web/#/mutti'}))
    credentials.chmod(0o600)
    print('READY:',root,flush=True)
    while all(p.poll() is None for p in processes):time.sleep(1)
except KeyboardInterrupt:
    print('Stopping this synthetic fixture; its data is retained.',flush=True)
finally:
    for p in reversed(processes):
        if p.poll() is None:
            p.terminate()
            try:p.wait(timeout=20)
            except subprocess.TimeoutExpired:p.kill();p.wait()
    for log in logs:log.close()
