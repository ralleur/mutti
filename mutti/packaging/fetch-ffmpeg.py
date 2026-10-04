#!/usr/bin/env python3
"""Fetch an upstream FFmpeg binary only after validating its pinned SHA-256."""
import hashlib, json, pathlib, subprocess, sys, tarfile
root = pathlib.Path(__file__).resolve().parents[2]
lock = json.loads((root / 'mutti/components.lock.json').read_text())
rid = sys.argv[1]
asset = lock['ffmpeg']['assets'][rid]
directory = root / 'build/ffmpeg' / rid
directory.mkdir(parents=True, exist_ok=True)
archive = directory / asset['archive']
if not archive.exists():
    partial = archive.with_suffix('.partial')
    subprocess.run(['curl', '--fail', '--location', '--retry', '2', lock['ffmpeg']['repository'] + '/releases/download/v' + lock['ffmpeg']['version'] + '/' + asset['archive'], '-o', str(partial)], check=True)
    partial.replace(archive)
if hashlib.sha256(archive.read_bytes()).hexdigest() != asset['sha256']:
    raise SystemExit('FFmpeg archive hash mismatch; refusing extraction')
with tarfile.open(archive) as tar:
    for name in ('ffmpeg', 'ffprobe'):
        member = tar.getmember(name)
        if not member.isfile(): raise SystemExit('Unexpected FFmpeg archive member')
        (directory / name).write_bytes(tar.extractfile(member).read())
        (directory / name).chmod(0o755)
print(directory)
