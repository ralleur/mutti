#!/usr/bin/env python3
"""One measurement day for P2 (inference heavy; run only when the fan may run).

Builds the Mac package (unless --skip-build), re-runs the v6 casting dev set
as a regression check (the harness changed after v6-dev-5; a regression stops
the session before any gate suite is seen), then runs the frozen product
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
parser.add_argument('--suites', default='qualification-real-de-v5.json,qualification-real-en-v3.json')
parser.add_argument('--skip-build', action='store_true')
parser.add_argument('--skip-holdout', action='store_true')
parser.add_argument('--skip-dev', action='store_true', help='skip the dev regression (only if it already passed on this build)')
parser.add_argument('--dev-min', type=int, default=60, help='dev cases that must pass (of 60)')
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
    for dotnet in (Path.home() / '.dotnet/dotnet', Path('/private/tmp/mutti-tools/dotnet/dotnet')):
        if 'MUTTI_DOTNET' not in env and dotnet.exists():
            env['MUTTI_DOTNET'] = str(dotnet)
    if run('build-mac', ['bash', 'mutti/packaging/build-mac.sh'], env) != 0:
        sys.exit('build failed')
cast = session / 'mutti-cast'
subprocess.call(['go', 'build', '-o', str(cast), './cmd/mutti-cast'], cwd=repo / 'mutti/hub')


def casting(name, fixture, repetitions):
    return run(name, [str(cast), '-fixture', f'mutti/tests/fixtures/{fixture}', '-rubric', 'docs/mutti/evidence/qualification-v6-protocol.md',
                      '-models', args.model, '-ollama', str(app / 'Contents/Resources/ai-engine/ollama'), '-store', str(args.models.parent),
                      '-output', str(session / name), '-repetitions', str(repetitions)])


if not args.skip_dev:
    casting('casting-v6-dev', 'model-casting-v6-dev.json', 1)
    summary = session / 'casting-v6-dev' / 'summary.json'
    passed = total = 0
    if summary.exists():
        for counts in json.loads(summary.read_text()).get(args.model, {}).get('categories', {}).values():
            p, n = counts.split('/')
            passed, total = passed + int(p), total + int(n)
    log(f'dev regression {passed}/{total}')
    if total == 0 or passed < args.dev_min:
        sys.exit(f'Dev regression {passed}/{total} below {args.dev_min}: fix the harness first. The gate suites stay unseen.')
for suite in args.suites.split(','):
    name = Path(suite).stem
    run(name, [sys.executable, 'mutti/tests/qualification-run.py', '--app', str(app), '--root', str(session / name), '--testenv', str(args.testenv),
               '--models', str(args.models), '--model', args.model, '--suite', f'mutti/tests/fixtures/{suite}', '--playback'])
if not args.skip_holdout:
    casting('casting-v6-holdout', 'model-casting-v6-holdout.json', 3)
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
