# Source and license inventory — development preview

| Component | Pinned source | License / distribution note |
| --- | --- | --- |
| Jellyfin Server | v12.1, `ee91c75e777da41a9c4f4855e70adc604fbf2ef8` | Retained GPL and file-specific notices; root LICENSE and original contributors retained. |
| Jellyfin Web | v12.1, `fae41f33eb7cd636a9ef68984adb82bb247a6e1b` plus pinned Mutti commit | GPL-2.0-or-later, LICENSE and UPSTREAM.md retained. |
| Intro Skipper | 12.0.4.0, `6e0cb179007ac4c16cd9f358e9a617e791e9bf06`, official DLL | GPL-3.0-only. Unmodified plugin; license and matching source archive included in `intro-skipper/`. Hashes in components.lock.json. |
| .NET | SDK 10.0.401; self-contained runtime | MIT and third-party notices supplied with runtime; inspect publish output. |
| Jellyfin FFmpeg | v8.1.3-1 on Mac; pinned upstream image on Docker | GPL build. Full matching source, build configuration and external-library sources required before redistributing a release. |
| Kurt artwork | Hauser private Kurt lab, `kurt-a-refined-11`; source hashes in `mutti/migrate/web/kurt/provenance.json` | Reused for the local Mutti preview at the owner's explicit request (2026-10-05). Original drawings repacked losslessly. No new public artwork license is inferred from this integration. |
| Sora | Copied from approved kurtz font assets | SIL Open Font License 1.1, bundled next to fonts. |
| Tailscale | v1.102.5 in isolated directlab | Experiment only. Not linked into the Mac app or container. Preserve upstream BSD/file-specific notices if adopted. |

Owned Mutti code is GPL-2.0-or-later. This inventory does not relicense upstream
files. Runtime/FFmpeg transitive dependency inventory and matching source bundles
remain release gates. Ad-hoc signed local builds are not a distributable release.

## Mutti Connect test transport

New common transport sources (`mutti/connect`) and Apple bridge code use MPL-2.0.
They are separate from the GPL-2.0-or-later Jellyfin server and Mac launcher.
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
