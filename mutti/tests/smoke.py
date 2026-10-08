#!/usr/bin/env python3
"""Destructive setup only in a fresh, explicitly named Mutti smoke instance."""
import argparse, json, pathlib, secrets, sys, time, urllib.request, urllib.error, urllib.parse

p=argparse.ArgumentParser()
p.add_argument('--url',required=True)
p.add_argument('--media-path',required=True)
p.add_argument('--sample',type=pathlib.Path,required=True)
p.add_argument('--fresh-smoke-instance',action='store_true',required=True)
args=p.parse_args()
assert args.url in ('http://127.0.0.1:18597','http://127.0.0.1:18598'), 'Only reserved local smoke ports are permitted'
owner_device='mutti-owner-'+secrets.token_hex(8)
player_device='mutti-playback-'+secrets.token_hex(8)
checks=[]
def request(path,method='GET',data=None,token=None,device=owner_device,extra=None):
    authorization=f'MediaBrowser Client="Mutti Smoke", Device="Synthetic test", DeviceId="{device}", Version="0.1.0"'
    if token: authorization+=f', Token="{token}"'
    headers={'Authorization':authorization,'Content-Type':'application/json'}
    headers.update(extra or {})
    req=urllib.request.Request(args.url+path, data=json.dumps(data).encode() if data is not None else (b'' if method=='POST' else None),headers=headers,method=method)
    try:
        with urllib.request.urlopen(req,timeout=10) as response:return response.status,response.headers,response.read()
    except urllib.error.HTTPError as e:return e.code,e.headers,e.read()
def api(path,method='GET',data=None,token=None,device=owner_device,expected=(200,204)):
    code,_,body=request(path,method,data,token,device)
    if code not in expected:raise AssertionError(f'{method} {path.split("?")[0]} returned {code}; response omitted to protect credentials')
    return json.loads(body) if body else None
def check(name,condition):
    if not condition:raise AssertionError(name)
    checks.append(name);print('PASS '+name,flush=True)

for _ in range(60):
    try:
        info=api('/System/Info/Public')
        if isinstance(info,dict) and 'StartupWizardCompleted' in info:break
        time.sleep(1)
    except (OSError,AssertionError):time.sleep(1)
else:raise SystemExit('Server did not become ready')
assert not info['StartupWizardCompleted'],'Refusing to modify an already configured instance'
check('Product server name',info['ServerName']=='Mutti')
check('Foreign Host rejected',request('/Startup/User',extra={'Host':'attacker.example'})[0]==403)
check('Foreign Origin rejected',request('/Startup/User',extra={'Origin':'https://attacker.example'})[0]==403)
check('Mutti web title',b'<title>Mutti</title>' in request('/web/index.html')[2])
api('/Startup/User')
api('/Startup/Configuration','POST',{'ServerName':'Mutti','UICulture':'en-US','MetadataCountryCode':'US','PreferredMetadataLanguage':'en'})
password=secrets.token_urlsafe(32)
api('/Startup/User','POST',{'Name':'Mutti Smoke Owner','Password':password})
api('/Startup/RemoteAccess','POST',{'EnableRemoteAccess':False})
auth=api('/Users/AuthenticateByName','POST',{'Username':'Mutti Smoke Owner','Pw':password})
token=auth['AccessToken']
check('Fresh owner setup and authentication',auth['User']['Policy']['IsAdministrator'])
player=api('/Users/New','POST',{'Name':'Mutti Smoke Playback','Password':secrets.token_urlsafe(32)},token)
check('Playback profile has no administrator rights',not player['Policy']['IsAdministrator'])
query=urllib.parse.urlencode({'name':'Mutti Smoke','collectionType':'movies','paths':args.media_path,'refreshLibrary':'true'})
api('/Library/VirtualFolders?'+query,'POST',{'LibraryOptions':{'EnableRealtimeMonitor':False,'EnableInternetProviders':False,'SaveLocalMetadata':False,'TypeOptions':[{'Type':'Movie','MetadataFetchers':[],'ImageFetchers':[]}]}},token)
api('/Startup/Complete','POST')
for _ in range(60):
    items=api('/Items?Recursive=true&IncludeItemTypes=Movie&Fields=Path',token=token)
    if items['Items']:break
    time.sleep(1)
else:raise AssertionError('Synthetic media was not indexed')
item=items['Items'][0]
check('Synthetic media library indexed',item['Path']==args.media_path.rstrip('/')+'/'+args.sample.name)
q=api('/QuickConnect/Initiate','POST',device=player_device)
check('Quick Connect waits for owner approval',not q['Authenticated'])
api('/QuickConnect/Authorize?'+urllib.parse.urlencode({'code':q['Code'],'userId':player['Id']}),'POST',token=token)
paired=api('/Users/AuthenticateWithQuickConnect','POST',{'Secret':q['Secret']},device=player_device)
play_token=paired['AccessToken']
check('Existing Jellyfin Quick Connect signs in chosen playback profile',paired['User']['Id']==player['Id'] and not paired['User']['Policy']['IsAdministrator'])
check('Playback profile cannot administer server',request('/System/Info/Storage',token=play_token,device=player_device)[0] in (401,403))
code,headers,body=request('/Videos/'+item['Id']+'/stream?static=true',token=play_token,device=player_device,extra={'Range':'bytes=0-4095'})
check('Authenticated video Range request',code==206 and headers.get('Content-Range','').startswith('bytes 0-4095/') and body==args.sample.read_bytes()[:4096])
api('/Devices?'+urllib.parse.urlencode({'id':player_device}),'DELETE',token=token)
check('Deleting a device rejects subsequent authenticated requests',request('/Users/Me',token=play_token,device=player_device)[0] in (401,403))
print(json.dumps({'result':'pass','checks':checks,'limitations':['synthetic 12-second media; no real movie playback','Quick Connect compatibility is not cryptographic enrollment','revocation checked on new requests, not active streams','no WAN, NAS hardware or Apple TV validation']},indent=2))
