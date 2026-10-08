#!/usr/bin/env python3
import json, os, pathlib, subprocess, sys
root = pathlib.Path(__file__).resolve().parents[2]
web = pathlib.Path(sys.argv[1]).resolve()
lock = json.loads((root / 'mutti/components.lock.json').read_text())
def git(directory, *args):
    return subprocess.check_output(['git', '-C', str(directory), *args], text=True).strip()
expected = lock['web']['commit']
dirty = bool(git(root, 'status', '--porcelain') or git(web, 'status', '--porcelain'))
if os.getenv('MUTTI_ALLOW_DIRTY') == '1':
    print('Development build: source changes are recorded in provenance, not release ready.')
else:
    if not expected or git(web, 'rev-parse', 'HEAD') != expected:
        raise SystemExit('Check out the Mutti Web commit in components.lock.json first.')
    if dirty: raise SystemExit('Source trees must be clean; MUTTI_ALLOW_DIRTY=1 is for local development only.')
