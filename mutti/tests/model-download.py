#!/usr/bin/env python3
"""Bounded model download into a dedicated test store.

Shows registry sizes first, enforces a byte budget, runs a separate engine on a
random loopback port with its own model directory and verifies every layer's
SHA-256 afterwards. Never touches the user's Ollama service or model store.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import time
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--model', action='append', required=True, help='library model, e.g. qwen3.5:9b')
parser.add_argument('--store', type=Path, required=True, help='new or dedicated test model directory')
parser.add_argument('--budget-gb', type=float, required=True)
args = parser.parse_args()

engine = shutil.which('ollama')
if not engine:
    raise SystemExit('ollama binary missing')
store = args.store.resolve()
if store == (Path.home() / '.ollama/models').resolve():
    raise SystemExit('Refusing the user model store')
store.mkdir(parents=True, exist_ok=True, mode=0o700)
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

def manifest(model):
    name, tag = model.split(':')
    if '/' in name or 'cloud' in tag:
        raise SystemExit('Only official local library models')
    req = urllib.request.Request(f'https://registry.ollama.ai/v2/library/{name}/manifests/{tag}',
                                 headers={'Accept': 'application/vnd.docker.distribution.manifest.v2+json'})
    with urllib.request.urlopen(req, timeout=30) as response:
        return json.load(response)

plan = {m: manifest(m) for m in args.model}
total = sum(l['size'] for d in plan.values() for l in d['layers'])
print(json.dumps({m: sum(l['size'] for l in d['layers']) for m, d in plan.items()}), 'total', total)
if total > args.budget_gb * 1e9:
    raise SystemExit('Download exceeds the approved budget')

with socket.socket() as probe:
    probe.bind(('127.0.0.1', 0))
    port = probe.getsockname()[1]
env = dict(os.environ, OLLAMA_HOST=f'127.0.0.1:{port}', OLLAMA_MODELS=str(store), OLLAMA_NO_CLOUD='1')
log = (store.parent / f'download-{int(time.time())}.log').open('w')
proc = subprocess.Popen([engine, 'serve'], env=env, stdout=log, stderr=log, start_new_session=True)
base = f'http://127.0.0.1:{port}/api/'
try:
    for _ in range(30):
        try:
            opener.open(base + 'version', timeout=2).read(); break
        except OSError:
            time.sleep(1)
    for model in args.model:
        req = urllib.request.Request(base + 'pull', data=json.dumps({'model': model, 'stream': True}).encode(),
                                     headers={'Content-Type': 'application/json'})
        last = ''
        with opener.open(req, timeout=3600) as response:
            for line in response:
                event = json.loads(line)
                if event.get('error'):
                    raise SystemExit(event['error'])
                status = event.get('status', '')
                if status != last:
                    print(model, status, flush=True); last = status
        for layer in plan[model]['layers']:
            blob = store / 'blobs' / layer['digest'].replace(':', '-')
            h = hashlib.sha256()
            with blob.open('rb') as f:
                for chunk in iter(lambda: f.read(1 << 22), b''):
                    h.update(chunk)
            if 'sha256:' + h.hexdigest() != layer['digest'] or blob.stat().st_size != layer['size']:
                raise SystemExit(f'Layer verification failed for {model}')
        (store.parent / f'manifest-{model.replace(":", "-")}.json').write_text(json.dumps(plan[model], indent=2))
        print(model, 'verified', flush=True)
finally:
    import signal
    try: os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError: pass
    proc.wait(timeout=20)
    log.close()
