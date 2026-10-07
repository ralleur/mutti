# Source and license inventory of the Mac package (07.10.2026)

Release gate M1/M6/P4 ("vollständiges Quell-/Lizenzinventar"). Purely
offline evaluation, no build, no legal review.

## Source state and commands

- Package: `build/macos/osx-arm64/Mutti.app` (worktree `mutti-p1`), built
  from server `e38a416dfa08`, web `9cde06c43bac`, both clean, channel
  `local-development`, Mutti 0.1.0-dev. 2,908 files in `Contents/Resources`.
- New scripts (branch `codex/mutti-p2-qualification`):
  - `mutti/packaging/inventory.py`: assigns every file to a component and
    reads licenses from local metadata: `server/jellyfin.deps.json` + NuGet
    cache, `go version -m` of the three Go programs + module cache,
    `package-lock.json`/`node_modules` of Mutti Web, FFmpeg build
    configuration (`ffmpeg -version`), `components.lock.json`. Reports open
    points by type.
  - `mutti/packaging/notices.py`: writes `licenses/THIRD-PARTY-NOTICES.txt`
    from the local license texts (.NET runtime packs incl. their
    THIRD-PARTY-NOTICES, NuGet, Go, npm).
  - Both are now part of `build-mac.sh` (notices before the component list,
    inventory as a report under `build/macos/<rid>/license-inventory.*`).
- Additionally: the MIT texts of llama.cpp (`b10380`) and mlx-c (`fba4470`) at
  exactly the revisions pinned by Ollama v0.32.13 (`LLAMA_CPP_VERSION`,
  `MLX_C_VERSION` of the tag) fetched once from GitHub and stored in the repo
  (`mutti/packaging/licenses/`, SHA-256 in `components.lock.json`).

```
python3 mutti/packaging/inventory.py --resources build/macos/osx-arm64/Mutti.app/Contents/Resources \
  --web ../mutti-web --json … --md …
python3 mutti/packaging/notices.py --resources <APFS clone of Resources> --web ../mutti-web
```

The existing package was not changed. The run "with notices" was done on an
APFS clone of `Resources` into which `notices.py` and the two new license
texts were added; this simulates the next build.

## Result

| Run | Files | Unassigned | Missing texts | Unknown license | Source obligation | Copyleft notes | Review |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| existing package | 2,908 | 0 | 4 | 6 | 3 | 7 | 9 |
| + notices + vendored texts (simulated) | 2,911 | 0 | 0 | 6 | 3 | 7 | 9 |

Components (size): Jellyfin server incl. .NET 194 MB (112 entries in
`deps.json`), web 58 MB (305 production npm packages), Ollama/llama.cpp/MLX
419 MB, FFmpeg 114 MB (30 external libraries), Go programs 45 MB (Connect 24
modules; hub and manager only the standard library), Intro Skipper 7 MB.
`THIRD-PARTY-NOTICES.txt`: 180 distinct license texts, 758 KB.

## Open points before distribution

1. **Unclear licenses (6, .NET):** DotNet.Glob 3.1.3, Ignore 0.2.1,
   LrcParser 2025.623.0, Morestachio 5.0.1.670, Ude.NetStandard 1.2.0,
   prometheus-net.DotNetRuntime 4.4.1. Their NuGet metadata contain no
   license (only project URLs, or "see license directory"). Determine from
   the upstream repositories; I deliberately did not fill these in from memory.
2. **Source obligations (3):** FFmpeg is GPL-3.0-or-later according to its
   build configuration (`--enable-gpl --enable-version3`, statically linked
   incl. x264/x265): complete corresponding source of FFmpeg, every linked
   library and the build scripts. Jellyfin server and web (GPL-2.0-or-later):
   source of exactly the built commits. Intro Skipper ships its source
   archive (checked).
3. **Copyleft libraries (7, keep notices/source availability):** BDInfo,
   TagLibSharp (LGPL-2.1-only), libse (LGPL-3.0), UTF.Unknown (MPL-1.1),
   @jellyfin/libass-wasm (LGPL-2.1+ et al.), @jellyfin/sdk and yamux
   (MPL-2.0). All unmodified upstream dependencies.
4. **Review (9):** Svg.Custom (MS-PL), six Noto fonts (OFL-1.1),
   `@jellyfin/ux-web` (**CC-BY-SA-4.0**, share-alike for these assets),
   argparse (Python-2.0).
5. **Limits of this evaluation:** npm production packages are approximated
   from `package-lock.json` (not from the bundler output). The licenses of
   FFmpeg's external libraries are known upstream licenses and must be
   checked against the sources. NuGet packages with only an SPDX expression
   are listed without a copyright line. No legal review.

Next steps: on the next package build (daytime) check the inventory as a
build report; clarify the six licenses; prepare a source offer/bundle for
FFmpeg, server and web; owner decision on the review cases.
