#!/usr/bin/env python3
"""Fresh synthetic Docker-package test; only its uniquely named container is removed."""
from pathlib import Path
import tempfile, subprocess, os,socket
root=Path(__file__).resolve().parents[2];work=Path(tempfile.mkdtemp(prefix='mutti-container-connect-'))
for port in (18593,18594,18597):
 with socket.socket() as sock:sock.bind(('127.0.0.1',port))
for name in ['config','cache','media','runner']:(work/name).mkdir(mode=0o700)
(work/'config/config').mkdir(mode=0o700)
(work/'config/config/network.xml').write_text('<NetworkConfiguration><InternalHttpPort>18597</InternalHttpPort><PublicHttpPort>18597</PublicHttpPort><EnableRemoteAccess>false</EnableRemoteAccess><AutoDiscovery>false</AutoDiscovery><EnableIPv6>false</EnableIPv6></NetworkConfiguration>')
buildenv=os.environ.copy();buildenv.update(GOOS='linux',GOARCH='arm64',CGO_ENABLED='0')
subprocess.run(['go','test','-c','-o',str(work/'runner/test-connect')],cwd=root/'mutti/connect',env=buildenv,check=True)
ffmpeg=root/'build/macos/osx-arm64/Mutti.app/Contents/Resources/ffmpeg/ffmpeg';sample=work/'media/Mutti Sample (2026).mp4'
subprocess.run([str(ffmpeg),'-f','lavfi','-i','testsrc2=size=640x360:rate=24','-t','12','-c:v','libx264','-pix_fmt','yuv420p','-an',str(sample)],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
name='mutti-connect-smoke-'+work.name.rsplit('-',1)[-1]
args=['docker','run','-d','--name',name,'--user',f'{os.getuid()}:{os.getgid()}','--read-only','--cap-drop=ALL','--security-opt=no-new-privileges:true','--tmpfs','/tmp:rw,noexec,nosuid,size=256m','-p','127.0.0.1:18597:18597','-p','127.0.0.1:18594:18594','-p','127.0.0.1:18593:18593','-v',f'{work}/config:/config','-v',f'{work}/cache:/cache','-v',f'{work}/media:/media:ro','-v',f'{work}/runner:/smoke:ro','--entrypoint','/mutti/connect','mutti:connect-preview','--state','/config/connect','--target','http://127.0.0.1:18597','--target-host','127.0.0.1:18597','--listen','0.0.0.0:18594','--admin-origin','http://127.0.0.1:18594','--lan','0.0.0.0:18593','--advertise','http://127.0.0.1:18593','--','/jellyfin/jellyfin','--webdir','/jellyfin/jellyfin-web','--ffmpeg','/usr/lib/jellyfin-ffmpeg/ffmpeg']
subprocess.run(args,check=True,stdout=subprocess.DEVNULL)
try:
 result=subprocess.run(['docker','exec','-e','MUTTI_FRESH_CONNECT_SMOKE=1','-e','MUTTI_SMOKE_MEDIA=/media','-e','MUTTI_SMOKE_SAMPLE=/media/Mutti Sample (2026).mp4','-e','MUTTI_CONNECT_SMOKE_URL=http://127.0.0.1:18597','-e','MUTTI_CONNECT_SMOKE_ADMIN=http://127.0.0.1:18594',name,'/smoke/test-connect','-test.run=TestRealJellyfin','-test.v','-test.count=1'])
 with (work/'container.log').open('w') as log:subprocess.run(['docker','logs',name],stdout=log,stderr=log)
finally:subprocess.run(['docker','rm','-f',name],stdout=subprocess.DEVNULL)
print('Disposable Docker artifacts:',work)
raise SystemExit(result.returncode)
