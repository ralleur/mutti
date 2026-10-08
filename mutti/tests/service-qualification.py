#!/usr/bin/env python3
"""Pinned, isolated Immich/Paperless API qualification. NOT a Mutti integration.

Creates only a new named Compose project and synthetic inputs. Existing services
and volumes are never adopted. Containers stop on exit; fixture data is retained.
No ML downloads: the Docker network is internal and all images must be present.
"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import time
import urllib.error
import urllib.request
import uuid
import zlib
import struct

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--root', type=Path, required=True)
args = parser.parse_args()
root = args.root.resolve()
if root.exists():
    raise SystemExit('Use a new directory: existing data is never accepted')
root.mkdir(parents=True, mode=0o700)
project = 'mutti-qualify-' + secrets.token_hex(5)
images = {
    'immich': 'ghcr.io/immich-app/immich-server@sha256:c15bff75068effb03f4355997d03dc7e0fc58720c2b54ad6f7f10d1bc57efaa5',
    'database': 'ghcr.io/immich-app/postgres@sha256:bcf63357191b76a916ae5eb93464d65c07511da41e3bf7a8416db519b40b1c23',
    'valkey': 'valkey/valkey@sha256:3b55fbaa0cd93cf0d9d961f405e4dfcc70efe325e2d84da207a0a8e6d8fde4f9',
    'paperless': 'ghcr.io/paperless-ngx/paperless-ngx@sha256:6c86cad803970ea782683a8e80e7403444c5bf3cf70de63b4d3c8e87500db92f',
    'redis': 'redis@sha256:3055dc25265b0c19ec90a1756dad4e0faff6f79e2557a6ac3d1274e39ee906f6',
}
for image in images.values():
    subprocess.run(['docker','image','inspect',image],check=True,stdout=subprocess.DEVNULL)
password = secrets.token_urlsafe(24)
base = {'restart':'no','pull_policy':'never'}
services = {
    'database':dict(base,image=images['database'],environment={'POSTGRES_PASSWORD':password,'POSTGRES_USER':'immich','POSTGRES_DB':'immich'},volumes=['database:/var/lib/postgresql/data']),
    'valkey':dict(base,image=images['valkey']),
    'immich':dict(base,image=images['immich'],volumes=['photos:/data'],environment={
        'DB_HOSTNAME':'database','DB_USERNAME':'immich','DB_PASSWORD':password,'DB_DATABASE_NAME':'immich','REDIS_HOSTNAME':'valkey',
        'IMMICH_MACHINE_LEARNING_ENABLED':'false'},depends_on=['database','valkey']),
    'redis':dict(base,image=images['redis']),
    'paperless':dict(base,image=images['paperless'],volumes=['documents:/usr/src/paperless/media','paperdata:/usr/src/paperless/data','export:/usr/src/paperless/export'],environment={
        'PAPERLESS_REDIS':'redis://redis:6379','PAPERLESS_SECRET_KEY':secrets.token_urlsafe(48),'PAPERLESS_ADMIN_USER':'testowner',
        'PAPERLESS_ADMIN_PASSWORD':password,'PAPERLESS_OCR_LANGUAGE':'eng','PAPERLESS_OCR_MODE':'skip','PAPERLESS_TIME_ZONE':'UTC',
        'PAPERLESS_URL':'http://127.0.0.1:30682','PAPERLESS_TASK_WORKERS':'1','PAPERLESS_THREADS_PER_WORKER':'1'},depends_on=['redis'])
}
restore_env=dict(services['paperless']['environment'])
restore_env.pop('PAPERLESS_ADMIN_USER');restore_env.pop('PAPERLESS_ADMIN_PASSWORD')
services['restored']=dict(base,image=images['paperless'],profiles=['restore'],environment=restore_env,
    volumes=['restore-documents:/usr/src/paperless/media','restore-data:/usr/src/paperless/data','export:/usr/src/paperless/export:ro'],depends_on=['redis'])
compose = {'name':project,'services':services,'volumes':{n:{} for n in ('database','photos','documents','paperdata','export','restore-documents','restore-data')},'networks':{'default':{'internal':True}}}
config=root/'compose.json';config.write_text(json.dumps(compose,indent=2));config.chmod(0o600)
cmd=['docker','compose','-p',project,'-f',str(config)]
report={'project':project,'images':images,'started':time.time(),'checks':[],'limitations':['API qualification, no Mutti adapter or native client','No HEIC/live-photo/background backup coverage','Text PDF only; OCR scan quality pending','Same Docker Desktop host; not physical NAS or native Mac packaging']}
def check(condition,message):
    if not condition:raise AssertionError(message)
    report['checks'].append(message);print('PASS:',message,flush=True)
def call(service,path,body=None,token='',method=None,raw=False,key=False):
    headers={'Content-Type':'application/json'}
    if token:
        if service=='photos':headers['x-api-key' if key else 'Authorization']=token if key else 'Bearer '+token
        else:headers['Authorization']='Token '+token
    if isinstance(body,tuple):data,headers['Content-Type']=body
    else:data=json.dumps(body).encode() if body is not None else None
    # Requests run inside this project's internal network. No service is exposed
    # to the LAN or to any other existing Docker project.
    url=('http://immich:2283' if service=='photos' else 'http://restored:8000' if service=='restored' else 'http://127.0.0.1:8000')+'/api'+path
    code='''import sys,json,base64,urllib.request,urllib.error
i=json.load(sys.stdin)
r=urllib.request.Request(i['url'],data=base64.b64decode(i['data']) if i['data'] is not None else None,headers=i['headers'],method=i['method'])
try:
 with urllib.request.urlopen(r,timeout=10) as response: print(json.dumps({'status':response.status,'body':base64.b64encode(response.read()).decode()}))
except urllib.error.HTTPError as error: print(json.dumps({'status':error.code,'body':base64.b64encode(error.read()).decode()}))
'''
    result=subprocess.run(cmd+['exec','-T','paperless','python','-c',code],input=json.dumps({'url':url,'data':base64.b64encode(data).decode() if data is not None else None,'headers':headers,'method':method}),text=True,capture_output=True,timeout=15)
    if result.returncode:raise OSError('Isolated API not ready: '+result.stderr[-300:])
    value=json.loads(result.stdout);data=base64.b64decode(value['body'])
    if value['status']>=400:raise urllib.error.HTTPError(url,value['status'],data.decode(errors='replace')[:300],{},None)
    return data if raw else json.loads(data) if data else None
def denied(service,path,token,key=False):
    try:call(service,path,token=token,key=key)
    except urllib.error.HTTPError as e:
        check(e.code in (400,401,403,404),'Cross-user access denied: '+service+path.split('?')[0]);return
    raise AssertionError('Foreign user gained access')
def multipart(field,name,content,mime,fields):
    boundary=uuid.uuid4().hex; parts=[]
    for k,v in fields.items():
        parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{k}"\r\n\r\n{v}\r\n'.encode())
    parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{field}"; filename="{name}"\r\nContent-Type: {mime}\r\n\r\n'.encode()+content+b'\r\n')
    parts.append(f'--{boundary}--\r\n'.encode())
    return b''.join(parts),'multipart/form-data; boundary='+boundary
def photo():
    def chunk(kind,data):return struct.pack('!I',len(data))+kind+data+struct.pack('!I',zlib.crc32(kind+data)&0xffffffff)
    pixels=b''.join(b'\0'+bytes([240,180,20])*64 for _ in range(64))
    return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('!IIBBBBB',64,64,8,2,0,0,0))+chunk(b'IDAT',zlib.compress(pixels))+chunk(b'IEND',b'')
def pdf():
    text=b'BT /F1 18 Tf 60 740 Td (Synthetic Mutti invoice RE-TEST-42 amount 123.45 EUR) Tj ET'
    objects=[b'<< /Type /Catalog /Pages 2 0 R >>',b'<< /Type /Pages /Kids [3 0 R] /Count 1 >>',b'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',b'<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',b'<< /Length '+str(len(text)).encode()+b' >>\nstream\n'+text+b'\nendstream']
    data=b'%PDF-1.4\n';offsets=[0]
    for i,obj in enumerate(objects,1):offsets.append(len(data));data+=str(i).encode()+b' 0 obj\n'+obj+b'\nendobj\n'
    start=len(data);data+=b'xref\n0 6\n0000000000 65535 f \n'+b''.join(f'{o:010d} 00000 n \n'.encode() for o in offsets[1:]);data+=f'trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n{start}\n%%EOF\n'.encode();return data
def wait_ready(service,path):
    for _ in range(120):
        try:call(service,path,raw=True);return
        except OSError:time.sleep(1)
    raise RuntimeError(service+' did not become ready')
try:
    with (root/'startup.log').open('w') as log:subprocess.run(cmd+['up','-d'],check=True,stdout=log,stderr=log)
    wait_ready('photos','/server/ping')
    call('photos','/auth/admin-sign-up',{'email':'owner@example.invalid','password':password,'name':'Test owner'})
    admin=call('photos','/auth/login',{'email':'owner@example.invalid','password':password})['accessToken']
    config_data=call('photos','/system-config',token=admin)
    config_data['machineLearning']['enabled']=False
    config_data['newVersionCheck']['enabled']=False
    call('photos','/system-config',config_data,admin,method='PUT')
    users=[];tokens=[]
    for name in ('alpha','beta'):
        user=call('photos','/admin/users',{'email':name+'@example.invalid','password':password,'name':'Test '+name,'shouldChangePassword':False,'notify':False},admin);users.append(user)
        tokens.append(call('photos','/auth/login',{'email':name+'@example.invalid','password':password})['accessToken'])
    content=photo();(root/'synthetic.png').write_bytes(content)
    upload=multipart('assetData','synthetic.png',content,'image/png',{'deviceAssetId':'fixture-1','deviceId':'mutti-test','fileCreatedAt':'2026-10-05T10:00:00.000Z','fileModifiedAt':'2026-10-05T10:00:00.000Z'})
    result=call('photos','/assets',upload,tokens[0]);asset=result['id']
    check(result['status']=='created','Immich accepts actual synthetic PNG')
    check(call('photos','/assets/'+asset,token=tokens[0])['ownerId']==users[0]['id'],'Asset owner matches uploading user')
    check(call('photos','/assets/'+asset+'/original',token=tokens[0],raw=True)==content,'Original download is byte-identical')
    denied('photos','/assets/'+asset,tokens[1]);denied('photos','/assets/'+asset+'/original',tokens[1])
    check(call('photos','/search/metadata',{'id':asset},tokens[1])['assets']['total']==0,'Metadata search does not reveal other user asset')
    check(call('photos','/assets',upload,tokens[0])['status']=='duplicate','Duplicate upload reuses existing original')
    scoped=call('photos','/api-keys',{'name':'Read-only qualification','permissions':['asset.read','asset.view','asset.download']},tokens[0])
    check(call('photos','/assets/'+asset,token=scoped['secret'],key=True)['id']==asset,'Scoped API key reads own asset')
    denied('photos','/admin/users',scoped['secret'],key=True)
    # Revoke only the synthetic key, preserving its original test photograph.
    call('photos','/api-keys/'+str(scoped['apiKey']['id']),token=tokens[0],method='DELETE')
    denied('photos','/assets/'+asset,scoped['secret'],key=True)
    wait_ready('docs','/schema/')
    # Isolated Django instance only: create two ordinary users with document model permissions.
    setup="from django.contrib.auth.models import User,Permission; from rest_framework.authtoken.models import Token; import json; users=[User.objects.create_user(username=n,password='fixture-unused') for n in ['alpha','beta']]; perms=Permission.objects.filter(codename__in=['view_document','add_document','change_document','view_paperlesstask']); [(u.user_permissions.set(perms)) for u in users]; print(json.dumps([{'id':u.id,'token':Token.objects.get_or_create(user=u)[0].key} for u in users]))"
    output=subprocess.check_output(cmd+['exec','-T','paperless','python','manage.py','shell','-c',setup],text=True)
    doc_users=json.loads(output.strip().splitlines()[-1])
    content=pdf();(root/'synthetic.pdf').write_bytes(content)
    uploaded=call('docs','/documents/post_document/',multipart('document','synthetic.pdf',content,'application/pdf',{'title':'Synthetic invoice RE-TEST-42','owner':doc_users[0]['id']}),doc_users[0]['token'])
    check(isinstance(uploaded,str),'Paperless returns asynchronous import task')
    for _ in range(120):
        tasks=call('docs','/tasks/?task_id='+uploaded,token=doc_users[0]['token'])
        if tasks and tasks[0]['status'] in ('SUCCESS','FAILURE'):break
        time.sleep(1)
    check(tasks and tasks[0]['status']=='SUCCESS','Document processing reaches SUCCESS')
    foreign_tasks=call('docs','/tasks/?task_id='+uploaded,token=doc_users[1]['token'])
    report['hazards']={'paperless_task_metadata_visible_to_other_user_with_task_permission':bool(foreign_tasks)}
    if foreign_tasks:print('FINDING: Paperless task API needs Mutti-owned job/profile filtering; do not expose it directly.',flush=True)
    found=call('docs','/documents/?query=RE-TEST-42',token=doc_users[0]['token'])
    check(found['count']==1,'Paperless full-text search finds processed invoice')
    document=found['results'][0];docid=document['id']
    check(document['owner']==doc_users[0]['id'] and '123.45' in document['content'],'Document text and owner are retained')
    denied('docs',f'/documents/{docid}/',doc_users[1]['token']);denied('docs',f'/documents/{docid}/download/',doc_users[1]['token'])
    check(call('docs','/documents/?query=RE-TEST-42',token=doc_users[1]['token'])['count']==0,'Document search respects user permissions')
    check(call('docs',f'/documents/{docid}/download/?original=true',token=doc_users[0]['token'],raw=True)==content,'Paperless original download is byte-identical')
    # Export is a consistent upstream export, not a raw copy of live SQLite.
    with (root/'export.log').open('w') as log:subprocess.run(cmd+['exec','-T','paperless','document_exporter','/usr/src/paperless/export'],stdout=log,stderr=log,check=True)
    check(True,'Upstream Paperless export completed')
    # Restore exclusively into new volumes, never over the running source.
    with (root/'restore.log').open('w') as log:
        subprocess.run(cmd+['up','-d','restored'],stdout=log,stderr=log,check=True)
        wait_ready('restored','/schema/')
        subprocess.run(cmd+['exec','-T','restored','document_importer','/usr/src/paperless/export'],stdout=log,stderr=log,check=True)
    restored_token=call('restored','/token/',{'username':'alpha','password':'fixture-unused'})['token']
    denied('restored',f'/documents/{docid}/',doc_users[0]['token'])
    check(call('restored',f'/documents/{docid}/download/?original=true',token=restored_token,raw=True)==content,'Independent Paperless restore preserves original bytes')
    restored_document=call('restored',f'/documents/{docid}/',token=restored_token)
    check(restored_document['owner']==doc_users[0]['id'] and '123.45' in restored_document['content'],'Independent restore retains document ownership and extracted text')
    restored_beta=call('restored','/token/',{'username':'beta','password':'fixture-unused'})['token']
    denied('restored',f'/documents/{docid}/',restored_beta)
    subprocess.run(cmd+['restart','immich','paperless'],check=True,stdout=subprocess.DEVNULL)
    wait_ready('photos','/server/ping');wait_ready('docs','/schema/')
    check(call('photos','/assets/'+asset+'/original',token=tokens[0],raw=True)==photo(),'Immich restart preserves original and session')
    check(call('docs',f'/documents/{docid}/download/?original=true',token=doc_users[0]['token'],raw=True)==pdf(),'Paperless restart preserves original and token')
    report['passed']=True
except Exception as error:
    report['passed']=False;report['error']=str(error)
    raise
finally:
    report['finished']=time.time()
    # The separately activated restore profile must also be included in cleanup.
    with (root/'services.log').open('w') as log:subprocess.run(cmd+['--profile','restore','logs','--no-color'],stdout=log,stderr=log)
    cleanup=subprocess.run(cmd+['--profile','restore','down'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    remaining=subprocess.check_output(['docker','ps','-aq','--filter','label=com.docker.compose.project='+project],text=True).strip()
    report['cleanup_completed']=cleanup.returncode==0 and not remaining
    if not report['cleanup_completed']:
        report['passed']=False
        report['cleanup_error']='The isolated fixture still has containers; inspect this project only.'
    (root/'result.json').write_text(json.dumps(report,indent=2)+'\n')
    if not report['cleanup_completed']:raise RuntimeError(report['cleanup_error'])
    print('Evidence and retained synthetic volumes:',root,flush=True)
