#!/usr/bin/env python3
"""Collect the license texts of bundled third-party code into the package.

Writes licenses/THIRD-PARTY-NOTICES.txt from local sources only: the .NET
runtime packs' LICENSE and THIRD-PARTY-NOTICES, the license files of NuGet
packages (or their license expression/URL), Go module license files from
the module cache for each bundled Go program, and the license files of the
web client's production npm packages. A package without a local text is
listed by name, version and license so the gap stays visible.

  notices.py --resources Mutti.app/Contents/Resources --web ../mutti-web
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import xml.etree.ElementTree as ET

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument('--resources', type=Path, required=True)
parser.add_argument('--web', type=Path, required=True)
args = parser.parse_args()
res = args.resources.resolve()
nuget = Path.home() / '.nuget' / 'packages'
gomod = Path(subprocess.check_output(['go', 'env', 'GOMODCACHE'], text=True).strip())
out = []
missing = []
texts = {}  # identical texts are printed once and referenced


def license_file(directory):
    if not directory.is_dir():
        return None
    for name in sorted(os.listdir(directory)):
        if re.match(r'(LICEN[SC]E|COPYING|NOTICE)(\.|$|-|_)', name, re.I) and (directory / name).is_file():
            return directory / name
    return None


def section(title, text):
    text = text.strip()
    key = hash(text)
    if key in texts:
        out.append(f'\n=== {title}\n(same license text as {texts[key]})\n')
        return
    texts[key] = title
    out.append(f'\n=== {title}\n\n{text}\n')


def unresolved(title, detail):
    missing.append(f'{title}: {detail}')
    out.append(f'\n=== {title}\n\nNo license text available locally ({detail}).\n')


# .NET runtime and server packages
deps = json.loads((res / 'server' / 'jellyfin.deps.json').read_text())
for key, lib in sorted(deps['libraries'].items()):
    if lib['type'] in ('project', 'reference'):
        continue  # Jellyfin itself: GPL, see Jellyfin.txt
    name, version = key.split('/', 1)
    package = name.removeprefix('runtimepack.')
    directory = nuget / package.lower() / version.lower()
    title = f'{package} {version} (.NET)'
    for extra in ('THIRD-PARTY-NOTICES.TXT', 'THIRD-PARTY-NOTICES.txt'):
        if (directory / extra).exists():
            section(f'{title}, third-party notices', (directory / extra).read_text(errors='replace'))
    f = license_file(directory)
    spec = directory / (package.lower() + '.nuspec')
    if f:
        section(title, f.read_text(errors='replace'))
    elif spec.exists():
        root = ET.parse(spec).getroot()
        ns = {'n': root.tag.split('}')[0].strip('{')}
        lic = root.find('.//n:license', ns)
        url = root.find('.//n:licenseUrl', ns)
        if lic is not None and lic.get('type') == 'expression':
            out.append(f'\n=== {title}\n\nLicense: {lic.text.strip()} (SPDX expression; standard text)\n')
        else:
            unresolved(title, 'license URL ' + url.text.strip() if url is not None and url.text else 'no license in the package metadata')
    else:
        unresolved(title, 'not in the local NuGet cache')

# Go programs


def escape(module):
    return re.sub(r'[A-Z]', lambda m: '!' + m.group(0).lower(), module)


goroot = Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip())
# Homebrew keeps the Go license one level above GOROOT.
golicense = next((p for p in (goroot / 'LICENSE', goroot.parent / 'LICENSE') if p.exists()), None)
section('Go runtime and standard library', golicense.read_text()) if golicense else unresolved('Go runtime and standard library', 'LICENSE not found next to GOROOT')
seen = set()
for binary in ('hub/mutti-hub', 'migrate/mutti-migrate', 'connect/mutti-connect'):
    info = subprocess.check_output(['go', 'version', '-m', str(res / binary)], text=True)
    for line in info.splitlines():
        parts = line.strip().split('\t')
        if parts[0] != 'dep' or (parts[1], parts[2]) in seen:
            continue
        seen.add((parts[1], parts[2]))
        f = license_file(gomod / f'{escape(parts[1])}@{parts[2]}')
        title = f'{parts[1]} {parts[2]} (Go, {binary.split("/")[0]})'
        section(title, f.read_text(errors='replace')) if f else unresolved(title, 'not in the module cache')

# Web client production packages
lock = json.loads((args.web / 'package-lock.json').read_text())
for path, meta in sorted(lock['packages'].items()):
    if not path or meta.get('dev') or meta.get('devOptional') or meta.get('link'):
        continue
    name = path.split('node_modules/')[-1]
    title = f'{name} {meta.get("version")} (web)'
    f = license_file(args.web / path)
    if f:
        section(title, f.read_text(errors='replace'))
    else:
        manifest = args.web / path / 'package.json'
        lic = json.loads(manifest.read_text()).get('license') if manifest.exists() else meta.get('license')
        if isinstance(lic, dict):
            lic = lic.get('type')
        if lic:
            out.append(f'\n=== {title}\n\nLicense: {lic} (declared in package.json; no license file in the package)\n')
        else:
            unresolved(title, 'no license file or declaration')

header = ('Third-party notices for this Mutti package\n\n'
          'Generated by mutti/packaging/notices.py from the exact package inputs.\n'
          'Jellyfin (GPL-2.0-or-later), Jellyfin Web, Intro Skipper, FFmpeg, Ollama, MLX\n'
          'and the Sora font have their own files in this directory.\n')
if missing:
    header += '\nEntries without a local text (to be resolved before a release):\n' + ''.join(f'- {m}\n' for m in missing)
target = res / 'licenses' / 'THIRD-PARTY-NOTICES.txt'
target.parent.mkdir(exist_ok=True)
target.write_text(header + ''.join(out))
print(f'{target}: {len(texts)} distinct texts, {len(missing)} without local text')
