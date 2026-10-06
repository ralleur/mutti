#!/usr/bin/env python3
"""Product qualification of a built Mutti.app on real adapters.

Starts a new synthetic instance from the app (module-package-smoke.py
--setup-only), then runs the app's own hub binary as `mutti-hub qualify`
with the app's bundled engine and a verified model store. Writes evidence
and unsigned candidates; signing is a separate, reviewed step.

  qualification-run.py --app build/macos/osx-arm64/Mutti.app \
      --root build/qualification-NEW --testenv .../module-testenv/private.json \
      --models .../store-mlx/models --model qwen3.8:27b-mlx \
      --suite mutti/tests/fixtures/qualification-real-de-v1.json
"""
import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import urllib.request

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
for name in ('--app', '--root', '--testenv', '--models', '--suite'):
    parser.add_argument(name, type=Path, required=True)
parser.add_argument('--model', required=True)
parser.add_argument('--repetitions', type=int, default=3)
args = parser.parse_args()
repo = Path(__file__).resolve().parents[2]
root = args.root.resolve()
if root.exists():
    raise SystemExit('Use a new directory')
resources = args.app.resolve() / 'Contents/Resources'
instance = root / 'instance'
root.mkdir(parents=True, mode=0o700)
log = (root / 'setup.log').open('w')
setup = subprocess.Popen([sys.executable, '-u', str(repo / 'mutti/tests/module-package-smoke.py'), '--app', str(args.app), '--root', str(instance),
                          '--testenv', str(args.testenv), '--setup-only'], stdout=subprocess.PIPE, stderr=log, text=True, start_new_session=True)
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
try:
    for line in setup.stdout:
        log.write(line)
        log.flush()
        if line.startswith('Setup ready'):
            break
    else:
        raise SystemExit('setup failed; see setup.log')
    fixture = json.loads((instance / 'fixture.json').read_text())
    tokens = {}
    for key, p in fixture['profiles'].items():
        req = urllib.request.Request(fixture['jellyfin'] + '/Users/AuthenticateByName', data=json.dumps({'Username': p['name'], 'Pw': p['password']}).encode(),
                                     headers={'Content-Type': 'application/json', 'Authorization': 'MediaBrowser Client="Qualification", Device="Synthetic", DeviceId="qualification", Version="0.1"'})
        with opener.open(req, timeout=60) as r:
            tokens[key] = json.loads(r.read())['AccessToken']
    for name, data in (('profiles.json', tokens), ('objects.json', fixture['objects'])):
        (root / name).write_text(json.dumps(data))
        (root / name).chmod(0o600)
    cmd = [str(resources / 'hub/mutti-hub'), 'qualify', '--state', fixture['hubState'], '--jellyfin', fixture['jellyfin'],
           '--jellyfin-host', fixture['jellyfin'].removeprefix('http://'), '--ollama', str(resources / 'ai-engine/ollama'),
           '--models', str(args.models.resolve()), '--model', args.model, '--suite', str(args.suite.resolve()),
           '--profiles', str(root / 'profiles.json'), '--objects', str(root / 'objects.json'), '--out', str(root / 'result'),
           '--repetitions', str(args.repetitions)]
    started = time.time()
    code = subprocess.call(cmd)
    print(f'qualify exit {code} after {time.time() - started:.0f}s; evidence in {root / "result"}', flush=True)
    sys.exit(code)
finally:
    try:
        os.killpg(setup.pid, signal.SIGINT)
        setup.wait(timeout=60)
    except Exception:
        try:
            os.killpg(setup.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
