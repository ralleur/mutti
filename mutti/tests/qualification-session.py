#!/usr/bin/env python3
"""One measurement day for P2 (inference heavy; run only when the fan may run).

Builds the Mac package (unless --skip-build), then runs the frozen product
suites on real adapters (German gate, English gate, with playback headroom
measured idle and under AI load), and the unseen v6 casting holdout. Writes
everything below build/qualification-session-<timestamp>/ and prints a
summary. Signs nothing.

  qualification-session.py --testenv .../module-testenv/private.json \
      --models .../store-mlx/models [--skip-build]
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import time

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument('--testenv', type=Path, required=True)
parser.add_argument('--models', type=Path, required=True, help='verified engine store models/ directory')
parser.add_argument('--model', default='qwen3.8:27b-mlx')
parser.add_argument('--suites', default='qualification-real-de-v3.json,qualification-real-en-v1.json')
parser.add_argument('--skip-build', action='store_true')
parser.add_argument('--skip-holdout', action='store_true')
args = parser.parse_args()
repo = Path(__file__).resolve().parents[2]
session = repo / 'build' / time.strftime('qualification-session-%Y%m%d-%H%M')
session.mkdir(parents=True)
app = repo / 'build/macos/osx-arm64/Mutti.app'
log = lambda msg: print(time.strftime('%H:%M:%S'), msg, flush=True)


def run(name, cmd, env=None):
    log(f'{name} ...')
    with (session / f'{name}.log').open('w') as out:
        code = subprocess.call(cmd, stdout=out, stderr=subprocess.STDOUT, env=env, cwd=repo)
    log(f'{name} exit {code}')
    return code


if not args.skip_build:
    env = dict(os.environ, MUTTI_WEB_DIR=os.environ.get('MUTTI_WEB_DIR', str(repo.parent / 'mutti-web')))
    if run('build-mac', ['bash', 'mutti/packaging/build-mac.sh'], env) != 0:
        sys.exit('build failed')
for suite in args.suites.split(','):
    name = Path(suite).stem
    run(name, [sys.executable, 'mutti/tests/qualification-run.py', '--app', str(app), '--root', str(session / name), '--testenv', str(args.testenv),
               '--models', str(args.models), '--model', args.model, '--suite', f'mutti/tests/fixtures/{suite}', '--playback'])
if not args.skip_holdout:
    subprocess.call(['go', 'build', '-o', str(session / 'mutti-cast'), './cmd/mutti-cast'], cwd=repo / 'mutti/hub')
    run('casting-v6-holdout', [str(session / 'mutti-cast'), '-fixture', 'mutti/tests/fixtures/model-casting-v6-holdout.json',
                               '-rubric', 'docs/mutti/evidence/qualification-v6-protocol.md', '-models', args.model,
                               '-ollama', str(app / 'Contents/Resources/ai-engine/ollama'), '-store', str(args.models.parent),
                               '-output', str(session / 'casting-v6-holdout'), '-repetitions', '3'])
print('\nSummary', session)
for suite in args.suites.split(','):
    summary = session / Path(suite).stem / 'result' / 'summary.json'
    if summary.exists():
        s = json.loads(summary.read_text())
        print(f'  {Path(suite).stem}:')
        for t in s['tasks']:
            print(f"    {t['task']:15} {t['passed']}/{t['runs']}  critical {t['criticalFailures']}  {'QUALIFIED' if t['qualified'] else 'not qualified'}  {'; '.join(t.get('reasons', []))}")
    for label in ('idle', 'ai-load'):
        p = session / Path(suite).stem / f'playback-{label}.json'
        if p.exists():
            print(f'    playback {label}: {p.read_text().strip()[:300]}')
holdout = session / 'casting-v6-holdout' / 'summary.json'
if holdout.exists():
    print('  holdout:', json.loads(holdout.read_text()))
