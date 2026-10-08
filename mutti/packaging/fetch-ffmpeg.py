#!/usr/bin/env python3
"""Fetch an upstream FFmpeg binary only after validating its pinned SHA-256.

The portable upstream archives contain only the two executables, so the licence
texts of the pinned tag are fetched separately and verified against their own
pinned hashes; the GPL requires them to accompany the binaries.
"""
import hashlib, json, pathlib, subprocess, sys, tarfile
root = pathlib.Path(__file__).resolve().parents[2]
lock = json.loads((root / 'mutti/components.lock.json').read_text())
rid = sys.argv[1]
asset = lock['ffmpeg']['assets'][rid]
directory = root / 'build/ffmpeg' / rid
directory.mkdir(parents=True, exist_ok=True)
release = lock['ffmpeg']['repository'] + '/releases/download/v' + lock['ffmpeg']['version'] + '/'
raw = lock['ffmpeg']['repository'].replace('https://github.com/', 'https://raw.githubusercontent.com/') + '/v' + lock['ffmpeg']['version'] + '/'


def fetch(url, target, sha256):
    if not target.exists():
        partial = target.with_suffix(target.suffix + '.partial')
        subprocess.run(['curl', '--fail', '--location', '--retry', '2', url, '-o', str(partial)], check=True)
        partial.replace(target)
    if hashlib.sha256(target.read_bytes()).hexdigest() != sha256:
        target.unlink()
        raise SystemExit(f'Hash mismatch for {target.name}; refusing to use it')


archive = directory / asset['archive']
fetch(release + asset['archive'], archive, asset['sha256'])
with tarfile.open(archive) as tar:
    for name in ('ffmpeg', 'ffprobe'):
        member = tar.getmember(name)
        if not member.isfile(): raise SystemExit('Unexpected FFmpeg archive member')
        (directory / name).write_bytes(tar.extractfile(member).read())
        (directory / name).chmod(0o755)
licenses = directory / 'licenses'
licenses.mkdir(exist_ok=True)
for name, sha256 in lock['ffmpeg']['licenses'].items():
    fetch(raw + name, licenses / name, sha256)
print(directory)
