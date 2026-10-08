#!/usr/bin/env python3
"""Copy the .NET runtime licence and third-party notices into the bundle.

A self-contained publish copies the runtime pack's files listed in its
RuntimeList.xml, which does not include LICENSE.TXT or THIRD-PARTY-NOTICES.TXT.
This reads the exact runtime pack version from the published deps.json, locates
the pack in the NuGet global packages folder and copies both files. Missing
notices fail the build: they are part of the licence inventory.
"""
import json, os, pathlib, shutil, subprocess, sys
rid, publish_dir, out_dir = sys.argv[1], pathlib.Path(sys.argv[2]), pathlib.Path(sys.argv[3])
dotnet = os.environ.get('MUTTI_DOTNET', 'dotnet')
deps_files = sorted(publish_dir.glob('*.deps.json'))
if not deps_files:
    raise SystemExit(f'No deps.json in {publish_dir}')
deps = json.loads(deps_files[0].read_text())
prefix = f'runtimepack.Microsoft.NETCore.App.Runtime.{rid}/'
versions = {key[len(prefix):] for target in deps.get('targets', {}).values() for key in target if key.startswith(prefix)}
if len(versions) != 1:
    raise SystemExit(f'Expected exactly one runtime pack for {rid} in {deps_files[0].name}, found {sorted(versions)}')
version = versions.pop()
listing = subprocess.run([dotnet, 'nuget', 'locals', 'global-packages', '--list'], check=True, capture_output=True, text=True).stdout
packages = pathlib.Path(listing.split(':', 1)[1].strip())
pack = packages / f'microsoft.netcore.app.runtime.{rid}' / version
out_dir.mkdir(parents=True, exist_ok=True)
for name, target in (('LICENSE.TXT', 'dotnet-LICENSE.txt'), ('THIRD-PARTY-NOTICES.TXT', 'dotnet-THIRD-PARTY-NOTICES.txt')):
    source = pack / name
    if not source.is_file():
        raise SystemExit(f'Missing {name} in runtime pack {pack}')
    shutil.copyfile(source, out_dir / target)
print(f'.NET runtime {version} notices copied from {pack}')
