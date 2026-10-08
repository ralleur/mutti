#!/usr/bin/env python3
"""Fresh synthetic Mutti instance for the opt-in Go end-to-end test. Never user data."""
from pathlib import Path
import tempfile, subprocess, os, urllib.request, socket
root=Path(__file__).resolve().parents[2]
with socket.socket() as probe:
    try: probe.bind(('127.0.0.1',18598))
    except OSError: raise SystemExit('Reserved smoke port 18598 is in use; refusing to continue')
work=Path(tempfile.mkdtemp(prefix='mutti-connect-smoke-'))
for name in ['data','config','cache','logs','media']: (work/name).mkdir(mode=0o700)
(work/'config/network.xml').write_text('<NetworkConfiguration><InternalHttpPort>18598</InternalHttpPort><PublicHttpPort>18598</PublicHttpPort><EnableRemoteAccess>false</EnableRemoteAccess><AutoDiscovery>false</AutoDiscovery><EnableIPv6>false</EnableIPv6><LocalNetworkAddresses><string>127.0.0.1</string></LocalNetworkAddresses></NetworkConfiguration>')
(work/'config/system.xml').write_text('<ServerConfiguration><ServerName>Mutti</ServerName></ServerConfiguration>')
ffmpeg=root/'build/macos/osx-arm64/Mutti.app/Contents/Resources/ffmpeg/ffmpeg'
sample=work/'media/Mutti Sample (2026).mp4'
subprocess.run([str(ffmpeg),'-f','lavfi','-i','testsrc2=size=640x360:rate=24','-t','12','-c:v','libx264','-pix_fmt','yuv420p','-an',str(sample)],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
args=[str(root/'build/server/osx-arm64/jellyfin'),'--datadir',str(work/'data'),'--configdir',str(work/'config'),'--cachedir',str(work/'cache'),'--logdir',str(work/'logs'),'--webdir',str(root.parent/'mutti-web/dist'),'--ffmpeg',str(ffmpeg)]
env=os.environ.copy();env.update(MUTTI_LOCAL_ONLY='1',MUTTI_PREVIEW_ORIGIN='http://127.0.0.1:18598')
with (work/'server.log').open('w') as log:
    child=subprocess.Popen(args,env=env,stdout=log,stderr=log)
    try:
        env.update(MUTTI_FRESH_CONNECT_SMOKE='1',MUTTI_SMOKE_MEDIA=str(work/'media'),MUTTI_SMOKE_SAMPLE=str(sample))
        result=subprocess.run(['go','test','-run','TestRealJellyfin','-v','-count=1'],cwd=root/'mutti/connect',env=env)
    finally:
        child.terminate()
        try: child.wait(timeout=15)
        except subprocess.TimeoutExpired: child.kill();child.wait()
print('Synthetic smoke artifacts:',work)
raise SystemExit(result.returncode)
