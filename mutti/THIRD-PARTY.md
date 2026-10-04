# Source and license inventory — development preview

| Component | Pinned source | License / distribution note |
| --- | --- | --- |
| Jellyfin Server | v12.1, `ee91c75e777da41a9c4f4855e70adc604fbf2ef8` | Retained GPL and file-specific notices; root LICENSE and original contributors retained. |
| Jellyfin Web | v12.1, `fae41f33eb7cd636a9ef68984adb82bb247a6e1b` plus pinned Mutti commit | GPL-2.0-or-later, LICENSE and UPSTREAM.md retained. |
| .NET | SDK 10.0.401; self-contained runtime | MIT and third-party notices supplied with runtime; inspect publish output. |
| Jellyfin FFmpeg | v8.1.3-1 on Mac; pinned upstream image on Docker | GPL build. Full matching source, build configuration and external-library sources required before redistributing a release. |
| Sora | Copied from approved kurtz font assets | SIL Open Font License 1.1, bundled next to fonts. |
| Tailscale | v1.102.5 in isolated directlab | Experiment only. Not linked into the Mac app or container. Preserve upstream BSD/file-specific notices if adopted. |

Owned Mutti code is GPL-2.0-or-later. This inventory does not relicense upstream
files. Runtime/FFmpeg transitive dependency inventory and matching source bundles
remain release gates. Ad-hoc signed local builds are not a distributable release.
