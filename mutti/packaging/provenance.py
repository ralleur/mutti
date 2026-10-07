#!/usr/bin/env python3
import json, pathlib, subprocess, sys
root = pathlib.Path(__file__).resolve().parents[2]
def info(path):
    def git(*args): return subprocess.check_output(['git', '-C', str(path), *args], text=True).strip()
    # commitTime orders builds of the same Jellyfin release: the update guard
    # never lets a package built from older sources open newer data.
    return {'commit': git('rev-parse', 'HEAD'), 'commitTime': int(git('log', '-1', '--format=%ct')), 'dirty': bool(git('status', '--porcelain'))}
result = {'product': 'Mutti', 'channel': 'local-development', 'releaseReady': False, 'server': info(root), 'web': info(sys.argv[1])}
pathlib.Path(sys.argv[2]).write_text(json.dumps(result, indent=2) + '\n')
