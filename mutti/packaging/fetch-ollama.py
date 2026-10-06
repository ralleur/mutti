#!/usr/bin/env python3
"""Fetch the pinned local AI engine for the Mac package.

The upstream archive must match the SHA-256 in components.lock.json (the same
value published in the release's sha256sum.txt). Universal files are thinned to
the package architecture and x86-only CPU kernels are dropped for arm64.
"""
import hashlib, json, pathlib, shutil, subprocess, sys, tarfile
root = pathlib.Path(__file__).resolve().parents[2]
lock = json.loads((root / 'mutti/components.lock.json').read_text())['aiEngine']
rid = sys.argv[1]
arch = {'osx-arm64': 'arm64', 'osx-x64': 'x86_64'}[rid]
asset = lock['assets'][rid]
cache = root / 'build/ollama-src'
cache.mkdir(parents=True, exist_ok=True)
archive = cache / asset['archive']
if not archive.exists():
    partial = archive.with_suffix('.partial')
    subprocess.run(['curl', '--fail', '--location', '--retry', '2', f"{lock['repository']}/releases/download/v{lock['version']}/{asset['archive']}", '-o', str(partial)], check=True)
    partial.replace(archive)
if hashlib.sha256(archive.read_bytes()).hexdigest() != asset['sha256']:
    raise SystemExit('AI engine archive hash mismatch; refusing extraction')
target = root / 'build/ollama' / rid
if target.exists():
    shutil.rmtree(target)
target.mkdir(parents=True)
with tarfile.open(archive) as tar:
    for member in tar.getmembers():
        name = pathlib.PurePosixPath(member.name)
        if name.is_absolute() or '..' in name.parts or member.issym() and pathlib.PurePosixPath(member.linkname).is_absolute():
            raise SystemExit('Unsafe engine archive member')
        if arch == 'arm64' and name.name.startswith('libggml-cpu-'):
            continue
        # MLX models are the Apple-silicon target (plan §1), so the arm64 package
        # ships the MLX runner kernels (mlx_metal_v3/v4); Intel Macs cannot use them.
        if name.name == 'llama-quantize' or arch != 'arm64' and name.parts[0].startswith('mlx_metal'):
            continue
        tar.extract(member, target, filter='data') if sys.version_info >= (3, 12) else tar.extract(member, target)
for path in target.rglob('*'):
    if path.is_file() and not path.is_symlink():
        info = subprocess.run(['file', '-b', str(path)], capture_output=True, text=True).stdout
        if 'universal binary' in info:
            subprocess.run(['lipo', str(path), '-thin', arch, '-output', str(path)], check=True)
shutil.copy(root / 'mutti/packaging/licenses/Ollama-MIT.txt', target / 'LICENSE-ollama.txt')
if arch == 'arm64':
    shutil.copy(root / 'mutti/packaging/licenses/MLX-MIT.txt', target / 'LICENSE-mlx.txt')
print(target)
