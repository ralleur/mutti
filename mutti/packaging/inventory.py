#!/usr/bin/env python3
"""Source and license inventory of a built Mutti Mac package (release gate).

Reads only local data: the package Resources, the server's .NET deps.json and
the local NuGet cache, the Go build information of the bundled Go programs
and the Go module cache, the web client's package-lock.json, FFmpeg's build
configuration and components.lock.json. Writes a JSON inventory and a
Markdown summary that lists every open point: unknown licenses, copyleft
source obligations, license texts missing from the package, and files that
belong to no known component.

  inventory.py --resources build/macos/osx-arm64/Mutti.app/Contents/Resources \\
      --web ../mutti-web --json build/license-inventory.json --md inventory.md

Licenses marked "from metadata" were read from package metadata or license
files; "known upstream" entries (FFmpeg's external libraries, whose sources
are not on this machine) must be confirmed against the sources before a
release. Nothing here is legal advice; it is the checklist for that review.
"""
import argparse
import collections
import json
import os
from pathlib import Path
import re
import subprocess
import xml.etree.ElementTree as ET

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument('--resources', type=Path, required=True)
parser.add_argument('--web', type=Path, required=True, help='mutti-web checkout (package-lock.json)')
parser.add_argument('--json', type=Path, required=True)
parser.add_argument('--md', type=Path, required=True)
args = parser.parse_args()
res = args.resources.resolve()
home = Path.home()
copyleft = re.compile(r'\b(A?GPL|LGPL|MPL|EPL|CDDL|OSL|EUPL)', re.I)
permissive = {'MIT', 'ISC', 'BSD-2-Clause', 'BSD-3-Clause', 'Apache-2.0', 'Zlib', '0BSD', 'Unlicense', 'CC0-1.0', 'BlueOak-1.0.0'}


def classify(expr):
    """permissive, copyleft or review for an SPDX expression; an OR with a
    permissive alternative is permissive (that alternative is chosen)."""
    e = expr.strip()
    while e.startswith('(') and e.endswith(')'):
        e = e[1:-1].strip()
    for op, pick in ((' OR ', any), (' AND ', all)):
        if op in e:
            kinds = [classify(x) for x in e.split(op)]
            if pick(k == 'permissive' for k in kinds):
                return 'permissive'
            return 'copyleft' if 'copyleft' in kinds else 'review'
    if e in permissive:
        return 'permissive'
    return 'copyleft' if copyleft.search(e) else 'review'
issues = []


def issue(kind, text):
    issues.append({'kind': kind, 'text': text})


def detect(text):
    """License family from a license file's text. Titles are taken from the
    head only: the MPL and GPL-3.0 texts mention other licenses further down."""
    t = ' '.join(text.split())
    head = t[:1200]
    for spdx, title in (('MPL-2.0', 'Mozilla Public License Version 2.0'), ('MPL-2.0', 'Mozilla Public License, version 2.0'),
                        ('AGPL-3.0', 'GNU AFFERO GENERAL PUBLIC LICENSE'), ('LGPL-3.0', 'GNU LESSER GENERAL PUBLIC LICENSE Version 3'),
                        ('LGPL-2.1', 'GNU LESSER GENERAL PUBLIC LICENSE Version 2.1'), ('GPL-3.0', 'GNU GENERAL PUBLIC LICENSE Version 3'),
                        ('GPL-2.0', 'GNU GENERAL PUBLIC LICENSE Version 2')):
        if title.lower() in head.lower():
            return spdx
    rules = [
        ('Apache-2.0', 'Apache License Version 2.0'),
        ('Apache-2.0', 'Apache License, Version 2.0'),
        ('MIT', 'Permission is hereby granted, free of charge'),
        ('ISC', 'Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee'),
        ('BSD-3-Clause', 'Neither the name'),
        ('BSD-2-Clause', 'Redistribution and use in source and binary forms'),
        ('Unlicense', 'This is free and unencumbered software released into the public domain'),
    ]
    found = [spdx for spdx, marker in rules if marker.lower() in t.lower()]
    if not found:
        return None
    # BSD-3 texts also contain the BSD-2 sentence; Apache texts mention MIT rarely.
    return found[0]


def license_file(directory):
    for name in sorted(os.listdir(directory)) if directory.is_dir() else []:
        if re.match(r'(LICEN[SC]E|COPYING)(\.|$|-)', name, re.I) and (directory / name).is_file():
            return directory / name
    return None


# --- files and components --------------------------------------------------

lock = json.loads((res / 'components.lock.json').read_text())
provenance = json.loads((res / 'build-provenance.json').read_text())
license_texts = sorted(p.name for p in (res / 'licenses').iterdir()) if (res / 'licenses').is_dir() else []
notices_path = res / 'licenses' / 'THIRD-PARTY-NOTICES.txt'
notices = notices_path.read_text(errors='replace') if notices_path.exists() else ''
components = collections.OrderedDict()


def component(key, name, prefixes, license, source, notes=''):
    components[key] = {'name': name, 'prefixes': prefixes, 'license': license, 'source': source, 'notes': notes,
                       'files': 0, 'bytes': 0, 'packages': []}


component('server', 'Jellyfin Server (Mutti fork) and .NET runtime', ['server/'], 'GPL-2.0-or-later (Jellyfin); MIT (.NET)',
          f"Jellyfin {lock['jellyfin']['version']}, Mutti commit {provenance['server']['commit'][:12]}" + (' (dirty)' if provenance['server']['dirty'] else ''))
component('web', 'Jellyfin Web (Mutti fork)', ['web/'], 'GPL-2.0-or-later', f"mutti-web {provenance['web']['commit'][:12]}" + (' (dirty)' if provenance['web']['dirty'] else ''))
component('hub', 'Mutti hub (module service)', ['hub/'], 'GPL-2.0-or-later', 'this repository, mutti/hub')
component('migrate', 'Mutti service manager', ['migrate/'], 'GPL-2.0-or-later', 'this repository, mutti/migrate')
component('connect', 'Mutti Connect', ['connect/'], 'MPL-2.0', 'this repository, mutti/connect')
component('export', 'Mutti export helper', ['export/'], 'GPL-2.0-or-later', 'this repository, mutti/export')
component('ai-engine', 'Ollama with llama.cpp/ggml and MLX', ['ai-engine/'], 'MIT', 'official Ollama release archive (components.lock.json)')
component('ffmpeg', 'Jellyfin FFmpeg (static)', ['ffmpeg/'], 'see FFmpeg section', 'jellyfin-ffmpeg release (fetch-ffmpeg.py)')
component('intro-skipper', 'Intro Skipper plugin', ['intro-skipper/'], 'GPL-3.0-only', 'official release DLL plus matching source archive')
component('licenses', 'License texts', ['licenses/'], '-', 'this package')
component('brand', 'Mutti brand assets and app localization', ['Mutti.icns', 'wordmark-light.png', 'de.lproj/', 'en.lproj/'], 'Mutti (own)', 'this repository')
component('fonts', 'Sora font', ['Sora-Bold.ttf'], 'OFL-1.1', 'kurtz font assets')
component('metadata', 'Package metadata', ['components.lock.json', 'build-provenance.json', 'components.json', 'components.sig'], '-', 'build')

unassigned = []
for path in sorted(res.rglob('*')):
    if path.is_dir() and not path.is_symlink():
        continue
    rel = path.relative_to(res).as_posix()
    for c in components.values():
        if any(rel == p or (p.endswith('/') and rel.startswith(p)) for p in c['prefixes']):
            c['files'] += 1
            c['bytes'] += path.lstat().st_size
            break
    else:
        unassigned.append(rel)
for rel in unassigned:
    issue('unassigned', f'`{rel}` belongs to no known component')

# --- .NET packages ------------------------------------------------------------

deps = json.loads((res / 'server' / 'jellyfin.deps.json').read_text())
nuget = home / '.nuget' / 'packages'
native = {}
for target in deps['targets'].values():
    for key, value in target.items():
        for asset in (value.get('native') or {}):
            native[Path(asset).name] = key
for key, lib in sorted(deps['libraries'].items()):
    name, version = key.split('/', 1)
    entry = {'name': name, 'version': version, 'kind': lib['type']}
    if lib['type'] == 'project':
        entry['license'], entry['origin'] = 'GPL-2.0-or-later', 'Jellyfin project'
    elif lib['type'] == 'reference':
        entry['license'], entry['origin'] = 'GPL-2.0-or-later', 'Jellyfin assembly reference'
    else:
        package = name.removeprefix('runtimepack.')
        directory = nuget / package.lower() / version.lower()
        spec = directory / (package.lower() + '.nuspec')
        entry['license'], entry['origin'] = None, 'nuspec'
        if spec.exists():
            root = ET.parse(spec).getroot()
            ns = {'n': root.tag.split('}')[0].strip('{')}
            lic = root.find('.//n:license', ns)
            url = root.find('.//n:licenseUrl', ns)
            if lic is not None and lic.get('type') == 'expression':
                entry['license'] = lic.text.strip()
            elif lic is not None and lic.get('type') == 'file':
                f = directory / lic.text.strip()
                entry['license'] = detect(f.read_text(errors='replace')) if f.exists() else None
                entry['origin'] = 'license file in package'
            project = root.find('.//n:projectUrl', ns)
            if project is not None and project.text:
                entry['project'] = project.text.strip()
            if entry['license']:
                pass
            elif url is not None and url.text:
                m = re.search(r'licenses\.nuget\.org/(.+)$', url.text.strip())
                entry['license'] = m.group(1) if m else None
                entry['licenseUrl'] = url.text.strip()
                if not m:
                    f = license_file(directory)
                    entry['license'] = detect(f.read_text(errors='replace')) if f else None
                    entry['origin'] = 'license URL; file' if f else 'license URL only'
        else:
            entry['origin'] = 'not in local NuGet cache'
        if entry['license'] is None:
            where = entry.get('licenseUrl') or entry.get('project')
            issue('unknown', f'.NET package {key}: no license in its metadata ({entry["origin"]}' + (f'; check {where}' if where else '') + ')')
    components['server']['packages'].append(entry)
for lib in sorted(p.name for p in (res / 'server').iterdir() if p.suffix in ('.dylib', '.so') or p.name in ('createdump',)):
    if lib not in native:
        issue('unassigned', f'native server file `{lib}` is not declared by any package in deps.json')
if 'Microsoft.NETCore.App.Runtime' not in notices:
    issue('missing-text', '.NET runtime THIRD-PARTY-NOTICES and MIT license are not in `licenses/` (run notices.py)')

# --- Go programs --------------------------------------------------------------

gomod = Path(subprocess.check_output(['go', 'env', 'GOMODCACHE'], text=True).strip())


def escape(module):
    return re.sub(r'[A-Z]', lambda m: '!' + m.group(0).lower(), module)


for key, binary in (('hub', 'hub/mutti-hub'), ('migrate', 'migrate/mutti-migrate'), ('connect', 'connect/mutti-connect')):
    path = res / binary
    if not path.exists():
        issue('missing', f'{binary} not in package')
        continue
    info = subprocess.check_output(['go', 'version', '-m', str(path)], text=True)
    goversion = info.splitlines()[0].split(':', 1)[1].strip()
    components[key]['packages'].append({'name': 'Go runtime and standard library', 'version': goversion, 'license': 'BSD-3-Clause', 'origin': 'Go distribution'})
    for line in info.splitlines():
        parts = line.strip().split('\t')
        if parts[0] != 'dep':
            continue
        module, version = parts[1], parts[2]
        directory = gomod / f'{escape(module)}@{version}'
        f = license_file(directory)
        spdx = detect(f.read_text(errors='replace')) if f else None
        components[key]['packages'].append({'name': module, 'version': version, 'license': spdx, 'origin': 'license file' if f else 'not in module cache'})
        if spdx is None:
            issue('unknown', f'Go module {module}@{version} in {binary}: license not determined')
if '=== Go runtime and standard library' not in notices:
    issue('missing-text', 'Go standard library (BSD-3-Clause) and Go module license texts are not in `licenses/` (run notices.py)')

# --- web client ---------------------------------------------------------------

lockfile = json.loads((args.web / 'package-lock.json').read_text())
for path, meta in sorted(lockfile['packages'].items()):
    if not path or meta.get('dev') or meta.get('devOptional') or meta.get('link'):
        continue
    name = path.split('node_modules/')[-1]
    lic, origin = meta.get('license'), 'package-lock.json'
    manifest = args.web / path / 'package.json'
    if not lic and manifest.exists():
        pkg = json.loads(manifest.read_text())
        lic, origin = pkg.get('license') or pkg.get('licenses'), 'node_modules package.json'
        if isinstance(lic, list):
            lic = ' OR '.join(x.get('type', '') if isinstance(x, dict) else str(x) for x in lic)
    if isinstance(lic, dict):
        lic = lic.get('type')
    components['web']['packages'].append({'name': name, 'version': meta.get('version'), 'license': lic, 'origin': origin})
    if not lic:
        issue('unknown', f'npm package {name}@{meta.get("version")}: no license in package-lock.json or package.json')
if '(web)' not in notices:
    issue('missing-text', 'license notices of the web client\'s npm packages are not in `licenses/` (run notices.py)')

# --- AI engine ----------------------------------------------------------------

engine = res / 'ai-engine'
vendored = lock.get('aiEngine', {}).get('vendored', {})
groups = [  # (title, files, license, revision, texts that must ship)
    ('Ollama', r'^ollama$', 'MIT', lock.get('aiEngine', {}).get('version', ''), ['ai-engine/LICENSE-ollama.txt']),
    ('llama.cpp / ggml (vendored by Ollama)', r'^(libggml|libllama|libmtmd|llama-server)', 'MIT',
     vendored.get('llama.cpp', {}).get('revision', ''), ['licenses/llama.cpp-MIT.txt']),
    ('MLX, mlx-c, JACCL (vendored by Ollama)', r'^mlx_metal_v\d', 'MIT',
     'mlx ' + vendored.get('mlx', {}).get('revision', '')[:12] + ', mlx-c ' + vendored.get('mlx-c', {}).get('revision', '')[:12],
     ['ai-engine/LICENSE-mlx.txt', 'licenses/mlx-c-MIT.txt']),
]
seen = set()
for title, pattern, spdx, revision, texts in groups:
    files = sorted(p.name for p in engine.iterdir() if re.match(pattern, p.name))
    seen.update(files)
    components['ai-engine']['packages'].append({'name': title, 'version': revision, 'license': spdx,
                                                'origin': 'components.lock.json', 'files': files})
    for text in texts:
        if not (res / text).exists():
            issue('missing-text', f'{title}: `{text}` not shipped')
for p in engine.iterdir():
    if p.name not in seen and not p.name.startswith('LICENSE'):
        issue('unassigned', f'AI engine file `{p.name}` not attributed to Ollama, llama.cpp/ggml or MLX')

# --- FFmpeg -------------------------------------------------------------------

ffmpeg_libs = {  # known upstream licenses; confirm against the sources
    'libx264': 'GPL-2.0-or-later', 'libx265': 'GPL-2.0-or-later', 'libzvbi': 'GPL-2.0-or-later (parts LGPL)',
    'libmp3lame': 'LGPL-2.0-or-later', 'libfribidi': 'LGPL-2.1-or-later', 'libbluray': 'LGPL-2.1-or-later',
    'gmp': 'LGPL-3.0-or-later or GPL-2.0-or-later', 'chromaprint': 'LGPL-2.1-or-later (parts MIT)',
    'libass': 'ISC', 'libfreetype': 'FTL or GPL-2.0', 'libharfbuzz': 'MIT', 'fontconfig': 'MIT-like (fontconfig)',
    'libxml2': 'MIT', 'openssl': 'Apache-2.0', 'lzma': 'public domain (xz)', 'zlib': 'Zlib', 'iconv': 'system (macOS)',
    'libvorbis': 'BSD-3-Clause', 'libopus': 'BSD-3-Clause', 'libtheora': 'BSD-3-Clause', 'libvpx': 'BSD-3-Clause',
    'libwebp': 'BSD-3-Clause', 'libdav1d': 'BSD-2-Clause', 'libsvtav1': 'BSD-3-Clause-Clear (AOM patent license)',
    'libopenmpt': 'BSD-3-Clause', 'libsrt': 'MPL-2.0', 'libzimg': 'WTFPL', 'opencl': 'system framework (macOS)',
    'audiotoolbox': 'system framework (macOS)', 'videotoolbox': 'system framework (macOS)',
}
version = subprocess.run([str(res / 'ffmpeg' / 'ffmpeg'), '-hide_banner', '-version'], capture_output=True, text=True).stdout
config = re.search(r'configuration: (.*)', version)
flags = config.group(1).split() if config else []
gpl, v3 = '--enable-gpl' in flags, '--enable-version3' in flags
binary_license = ('GPL-3.0-or-later' if v3 else 'GPL-2.0-or-later') if gpl else ('LGPL-3.0-or-later' if v3 else 'LGPL-2.1-or-later')
components['ffmpeg']['license'] = binary_license + ' (from build configuration)'
components['ffmpeg']['source'] = version.splitlines()[0] if version else 'ffmpeg -version failed'
for flag in flags:
    m = re.match(r'--enable-(lib\w+|gmp|openssl|lzma|zlib|iconv|fontconfig|opencl|chromaprint|audiotoolbox|videotoolbox)$', flag)
    if m:
        lib = m.group(1)
        components['ffmpeg']['packages'].append({'name': lib, 'license': ffmpeg_libs.get(lib), 'origin': 'known upstream'})
        if lib not in ffmpeg_libs:
            issue('unknown', f'FFmpeg external library {lib}: license not in the known list')
issue('source', f'FFmpeg is {binary_license}: the complete corresponding source of FFmpeg, every statically linked library and the build scripts must ship with or be offered for the release')

# --- source obligations and summary -----------------------------------------

intro = res / 'intro-skipper'
if not any(p.suffix in ('.zip', '.gz') and 'source' in p.name.lower() for p in intro.iterdir()):
    issue('source', 'Intro Skipper: matching source archive not found in `intro-skipper/`')
for key in ('server', 'web'):
    issue('source', f'{components[key]["name"]} (GPL): corresponding source of exactly this build ({components[key]["source"]}) must be published or offered')
for c in components.values():
    for p in c['packages']:
        if p.get('kind') in ('project', 'reference') or c['name'] == 'Jellyfin FFmpeg (static)':
            continue  # the component's own code; covered by the source obligation
        kind = classify(p['license']) if p.get('license') else None
        if kind == 'copyleft':
            issue('copyleft', f'{c["name"]}: {p["name"]} {p.get("version", "")} is {p["license"]}; keep notices and source availability')
        elif kind == 'review':
            issue('review', f'{c["name"]}: {p["name"]} {p.get("version", "")} is {p["license"]}; check the conditions for distribution')

inventory = {'package': str(res), 'product': lock.get('version'), 'provenance': provenance, 'licenseTexts': license_texts,
             'components': list(components.values()), 'issues': issues}
args.json.parent.mkdir(parents=True, exist_ok=True)
args.json.write_text(json.dumps(inventory, indent=2) + '\n')

counts = collections.Counter(i['kind'] for i in issues)
md = [f'# Package inventory ({lock.get("version")}, generated)', '',
      f'Package: Mutti {lock.get("version")}, server `{provenance["server"]["commit"][:12]}`'
      + (' (dirty)' if provenance['server']['dirty'] else '') + f', web `{provenance["web"]["commit"][:12]}`'
      + (' (dirty)' if provenance['web']['dirty'] else '') + f', channel `{provenance.get("channel")}`.', '',
      '| Component | Files | MB | License | Source | Packages |', '| --- | ---: | ---: | --- | --- | ---: |']
for c in components.values():
    md.append(f'| {c["name"]} | {c["files"]} | {c["bytes"] / 1e6:.1f} | {c["license"]} | {c["source"]} | {len(c["packages"])} |')
md += ['', '## Licenses of bundled packages', '']
for c in components.values():
    if not c['packages']:
        continue
    lic = collections.Counter(p.get('license') or 'UNKNOWN' for p in c['packages'])
    md.append(f'- **{c["name"]}:** ' + ', '.join(f'{k} {v}' for k, v in lic.most_common()))
md += ['', '## Open points', '', f'{len(issues)} in total: ' + ', '.join(f'{k} {v}' for k, v in sorted(counts.items())), '']
for kind in ('unassigned', 'unknown', 'missing-text', 'source', 'copyleft', 'review', 'missing'):
    items = [i['text'] for i in issues if i['kind'] == kind]
    if items:
        md.append(f'### {kind}')
        md.append('')
        md += [f'- {t}' for t in items]
        md.append('')
args.md.write_text('\n'.join(md))
print(f'{sum(c["files"] for c in components.values())} files, {len(unassigned)} unassigned; '
      + ', '.join(f'{k} {v}' for k, v in sorted(counts.items())))
