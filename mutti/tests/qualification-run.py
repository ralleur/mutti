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
import threading
import time
import urllib.request

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
for name in ('--app', '--root', '--testenv', '--models', '--suite'):
    parser.add_argument(name, type=Path, required=True)
parser.add_argument('--model', required=True)
parser.add_argument('--repetitions', type=int, default=3)
parser.add_argument('--playback', action='store_true', help='measure playback headroom idle and under the AI load of this run')
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
    probe = [sys.executable, str(repo / 'mutti/tests/playback-under-load.py'), '--fixture', str(instance / 'fixture.json'),
             '--ffmpeg', str(resources / 'ffmpeg/ffmpeg')]

    def playback(label):
        with (root / f'playback-{label}.json').open('w') as out:
            subprocess.call(probe + ['--label', label], stdout=out, stderr=subprocess.STDOUT)

    if args.playback:
        playback('idle')

    def under_load():
        results = root / 'result' / 'results.jsonl'
        while qualify.poll() is None:
            if results.exists() and len(results.read_text().splitlines()) >= 3:
                playback('ai-load')
                return
            time.sleep(5)

    started = time.time()
    qualify = subprocess.Popen(cmd)
    watcher = threading.Thread(target=under_load, daemon=True)
    if args.playback:
        watcher.start()
    code = qualify.wait()
    if args.playback:
        watcher.join(timeout=600)
    print(f'qualify exit {code} after {time.time() - started:.0f}s; evidence in {root / "result"}', flush=True)
    sys.exit(code)
finally:
    # The setup process stops the services it started when it gets SIGINT
    # (its own clean-up). Signal it directly: macOS may refuse a signal to
    # the whole process group (EPERM), which left instances running before.
    try:
        setup.send_signal(signal.SIGINT)
        setup.wait(timeout=90)
    except subprocess.TimeoutExpired:
        setup.terminate()
        try:
            setup.wait(timeout=30)
        except subprocess.TimeoutExpired:
            setup.kill()
    except ProcessLookupError:
        pass
    # Defensive: services of this instance that still listen on the fixed
    # test ports (seen when the setup's own clean-up did not run) are
    # stopped by their process group, but only if they belong to it.
    for port in (32593, 32594, 32595, 32596, 32600):
        found = subprocess.run(['lsof', '-nP', '-t', f'-iTCP:{port}', '-sTCP:LISTEN'], capture_output=True, text=True).stdout.split()
        for pid in found:
            command = subprocess.run(['ps', '-o', 'command=', '-p', pid], capture_output=True, text=True).stdout
            if str(root) in command or (port == 32600 and str(resources) in command):
                try:
                    os.killpg(os.getpgid(int(pid)), signal.SIGTERM)
                except (ProcessLookupError, PermissionError):
                    pass
    for _ in range(60):
        if not any(subprocess.run(['lsof', '-nP', '-t', f'-iTCP:{port}', '-sTCP:LISTEN'], capture_output=True, text=True).stdout.strip()
                   for port in (32593, 32594, 32595, 32596, 32600)):
            break
        time.sleep(1)
