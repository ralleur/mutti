# Source and license inventory — development preview

| Component | Pinned source | License / distribution note |
| --- | --- | --- |
| Jellyfin Server | v12.1, `ee91c75e777da41a9c4f4855e70adc604fbf2ef8` | GPL. Upstream's own artefacts disagree on the version: the root LICENSE is the GPL v2 text, eight NuGet projects declare GPL-3.0-only, and upstream packaging treats the source as "GPL unversioned" with binaries under GPL v3. Root LICENSE, notices and contributors retained; see docs/mutti/licensing.md. |
| Jellyfin Web | v12.1, `fae41f33eb7cd636a9ef68984adb82bb247a6e1b` plus pinned Mutti commit | GPL v2 text as LICENSE; package.json declares GPL-2.0-or-later. LICENSE and UPSTREAM.md retained. |
| Intro Skipper | 12.0.4.0, `6e0cb179007ac4c16cd9f358e9a617e791e9bf06`, official DLL | GPL-3.0-only. Unmodified plugin; the build fetches DLL, LICENSE and matching source archive with pinned hashes (`packaging/fetch-intro-skipper.go`) into the bundle's `intro-skipper/` resources, and the LICENSE is also copied to `licenses/`. |
| .NET | SDK 10.0.401; self-contained runtime | MIT. A self-contained publish does not copy the runtime pack's LICENSE.TXT and THIRD-PARTY-NOTICES.TXT; `packaging/collect-dotnet-notices.py` copies them into `licenses/` and fails the build if they are missing. |
| Jellyfin FFmpeg | v8.1.3-1 on Mac; pinned upstream image on Docker | GPL version 3 build (`--enable-gpl --enable-version3`). The portable archive carries no licence text, so COPYING.GPLv3 and LICENSE.md of the tag are fetched with pinned hashes and bundled. Matching source (fork tag, builder scripts, pinned third-party library sources) is still required before redistributing a release. |
| Sora | Copied from approved kurtz font assets | SIL Open Font License 1.1, bundled next to fonts. |
| Tailscale | v1.102.5 in isolated directlab | Experiment only. Not linked into the Mac app or container. Preserve upstream BSD/file-specific notices if adopted. |

Owned Mutti code is GPL-2.0-or-later. This inventory does not relicense upstream
files. The licence under which the combined package is offered is an open owner
decision recorded in docs/mutti/licensing.md; the FFmpeg matching-source bundle
and the transitive dependency inventory remain release gates. Ad-hoc signed
local builds are not a distributable release.

## Mutti Connect test transport

New common transport sources (`mutti/connect`) and Apple bridge code use MPL-2.0.
They are separate files next to the GPL-licensed Jellyfin server and the GPL-2.0-or-later Mac launcher; MPL-2.0 section 3.3 allows this larger work, and none of the MPL files carries an Exhibit B notice.
Pinned dependency graph: `mutti/connect/go.mod` and `go.sum`. Direct components:
Pion WebRTC v4.2.22 and its Pion transport libraries (MIT), HashiCorp yamux v0.1.2
(MPL-2.0), skip2/go-qrcode da1b6568686e (MIT), Go x/time v0.14.0 (BSD-3-Clause).
Sora is served locally, with its OFL in `mutti/connect/Sora-OFL.txt`.
Pion's transitive TURN package is compiled as part of ICE; no TURN server is
configured or instantiated, and relay candidates are rejected. A complete
redistributable license/source bundle remains a release gate.

## Curated Intro Skipper

[Upstream](https://github.com/intro-skipper/intro-skipper/tree/6e0cb179007ac4c16cd9f358e9a617e791e9bf06)
remains unmodified. The reproducible fetch script verifies release ZIP, DLL and
matching source archive independently. The GPL-3.0-only plugin keeps its own
copyright/license; Mutti's bridge/snapshot utility remains GPL-2.0-or-later.
The official plugin uses the server's existing Jellyfin/.NET/SQLite runtime;
its embedded UI and source notices travel with the original DLL/source archive.
