# Lizenzprüfung vor der ersten Distribution

Stand: 7. Oktober 2026. Dieses Dokument hält die mit Primärquellen belegten
Lizenzfakten der gebündelten Komponenten fest, benennt die offenen Fragen und
definiert das Release-Gate. Es ist **keine Rechtsberatung** und trifft keine
Lizenzentscheidung; die Entscheidung über die Außenlizenz des Pakets bleibt beim
Owner, gegebenenfalls mit anwaltlicher Prüfung. Alle Quellen wurden am 7. Oktober
2026 gelesen und in einer zweiten Runde gegengeprüft.

## 1. Belegte Fakten

### Jellyfin Server

| Fakt | Quelle |
| --- | --- |
| Die Datei `LICENSE` ist der unveränderte Text der GNU GPL **Version 2**, ohne projektspezifischen „only“- oder „or later“-Hinweis. Sie stammt aus der Emby-Zeit (2013) und ist bis v12.1 byteidentisch. | [LICENSE](https://raw.githubusercontent.com/jellyfin/jellyfin/master/LICENSE), [Umbenennung 2018](https://github.com/jellyfin/jellyfin/commit/cc2878270) |
| README-Badge und GitHub-Lizenzfeld sagen „GPL 2.0“; sie leiten sich nur aus der Datei ab. | [README](https://raw.githubusercontent.com/jellyfin/jellyfin/master/README.md) |
| **Acht** NuGet-Projektdateien deklarieren `PackageLicenseExpression` **GPL-3.0-only**: Emby.Naming, Jellyfin.Data, MediaBrowser.Common, MediaBrowser.Controller, MediaBrowser.Model, Jellyfin.Database.Implementations, Jellyfin.Extensions, Jellyfin.MediaEncoding.Keyframes. nuget.org zeigt die Pakete 12.1.0 und 12.2.0 als GPL-3.0-only. | [csproj](https://github.com/jellyfin/jellyfin/blob/master/MediaBrowser.Common/MediaBrowser.Common.csproj), [nuget.org](https://www.nuget.org/packages/Jellyfin.Common) |
| Historie: bis 25.08.2020 verwiesen die Projektdateien auf die GPL-2.0-URL; am 26.08.2020 wurde erst GPL-3.0-or-later, dann am selben Tag GPL-3.0-only eingetragen (PR #3988 „Use proper SPDX Identifier“, ohne Beschreibung oder Begründung). | [670c41ee8](https://github.com/jellyfin/jellyfin/commit/670c41ee8), [5f60da29](https://github.com/jellyfin/jellyfin/commit/5f60da29c737cae4ee298f9fbeae971740f8a5ba), [b02650ec](https://github.com/jellyfin/jellyfin/commit/b02650ec2f812500e99c03a80425d548fc5cfc0c), [PR #3988](https://github.com/jellyfin/jellyfin/pull/3988) |
| Die ausdrücklichste Maintainer-Aussage (joshuaboniface, 01.04.2021): Die GPL-Version des ursprünglichen Emby-Codes ist unbekannt, das Projekt behandelt den Quellcode als „unversioned“; wegen GPLv3-only-kompatibler Abhängigkeiten müssten die Binaries GPLv3 sein. | [PR #5663](https://github.com/jellyfin/jellyfin/pull/5663) |
| Das offizielle Packaging-Repository deklariert in `debian/copyright` den Code als „GPL unversioned“ und die Binaries als GPL Version 3 only. | [jellyfin-packaging debian/copyright](https://raw.githubusercontent.com/jellyfin/jellyfin-packaging/master/debian/copyright) |
| Das frühere `debian/copyright` im Server-Repository (2018 bis 2024) nannte GPL-2.0+ mit „or later“; es wurde 2024 entfernt. | [v10.7.0 debian/copyright](https://raw.githubusercontent.com/jellyfin/jellyfin/v10.7.0/debian/copyright) |
| Issue #16653 „Licensing conflicts in Jellyfin“ (April 2026) benennt genau den Widerspruch GPL-2.0-Text gegen GPL-3.0-only-Pakete; es wurde als „not planned“ geschlossen. Die Kommentare konnten nicht gelesen werden. | [Issue #16653](https://github.com/jellyfin/jellyfin/issues/16653) |
| Quelldateien tragen keine SPDX- oder Lizenzkopfzeilen; `AssemblyInfo.cs` nennt die GPL ohne Version. Ein Vorschlag für GPLv2+/MPLv2-Kopfzeilen (PR #546, 2019) wurde abgelehnt. | [Program.cs](https://raw.githubusercontent.com/jellyfin/jellyfin/master/Jellyfin.Server/Program.cs), [PR #546](https://github.com/jellyfin/jellyfin/pull/546) |
| Das offizielle Plugin-Template ist GPLv3; sein README sagt, kompilierte Plugins linken gegen die GPLv3-NuGet-Pakete und werden dadurch GPLv3; proprietäre Plugins seien nicht zulässig. | [Plugin-Template README](https://raw.githubusercontent.com/jellyfin/jellyfin-plugin-template/master/README.md), [PR #17](https://github.com/jellyfin/jellyfin-plugin-template/pull/17) |
| Upstream liefert in keinem Paket (Docker, Debian) ein Plugin mit; Plugins werden zur Laufzeit aus dem Katalog geladen. | [packaging Dockerfile](https://raw.githubusercontent.com/jellyfin/jellyfin-packaging/master/docker/Dockerfile), [debian/control](https://raw.githubusercontent.com/jellyfin/jellyfin-packaging/master/debian/control) |

### Jellyfin Web

| Fakt | Quelle |
| --- | --- |
| `LICENSE` ist ebenfalls der reine GPLv2-Text. `package.json` deklariert **GPL-2.0-or-later** (seit 2019, Commit „Fix invalid license“, ohne Begründung). Keine Kopfzeilen in den Quelldateien. | [LICENSE](https://raw.githubusercontent.com/jellyfin/jellyfin-web/master/LICENSE), [package.json](https://raw.githubusercontent.com/jellyfin/jellyfin-web/master/package.json) |

### Intro Skipper 12.0.4.0

| Fakt | Quelle |
| --- | --- |
| `LICENSE` am gepinnten Commit ist der unveränderte GPLv3-Text; README: „licensed under the GNU General Public License v3.0“; Quelldateien tragen `SPDX-License-Identifier: GPL-3.0-only`. Das Projekt kompiliert gegen die Jellyfin-NuGet-Pakete 12.1 (GPL-3.0-only). | [LICENSE](https://raw.githubusercontent.com/intro-skipper/intro-skipper/6e0cb179007ac4c16cd9f358e9a617e791e9bf06/LICENSE), [Plugin.cs](https://raw.githubusercontent.com/intro-skipper/intro-skipper/12.0/IntroSkipper/Plugin.cs), [csproj](https://raw.githubusercontent.com/intro-skipper/intro-skipper/12.0/IntroSkipper/IntroSkipper.csproj) |
| Intro Skipper ist nicht im offiziellen Jellyfin-Katalog; es wird über ein eigenes Repository verteilt. | [jellyfin-meta-plugins](https://raw.githubusercontent.com/jellyfin/jellyfin-meta-plugins/master/.gitmodules) |

### Jellyfin FFmpeg 8.1.3-1

| Fakt | Quelle |
| --- | --- |
| FFmpeg ist standardmäßig LGPL v2.1+; `--enable-gpl` macht den Build GPL, `--enable-version3` wählt **Version 3**. Die portablen „gpl“-Varianten (auch mac64/macarm64) setzen beide Schalter; der Builder benennt `COPYING.GPLv3` als Lizenzdatei. | [LICENSE.md](https://raw.githubusercontent.com/jellyfin/jellyfin-ffmpeg/v8.1.3-1/LICENSE.md), [defaults-gpl.sh](https://raw.githubusercontent.com/jellyfin/jellyfin-ffmpeg/v8.1.3-1/builder/variants/defaults-gpl.sh), [macarm64-gpl.sh](https://raw.githubusercontent.com/jellyfin/jellyfin-ffmpeg/v8.1.3-1/builder/variants/macarm64-gpl.sh) |
| Das portable Mac-Archiv (SHA-256 wie in `components.lock.json`) enthält **nur** `ffmpeg` und `ffprobe`, keinen Lizenztext. | [Release v8.1.3-1](https://github.com/jellyfin/jellyfin-ffmpeg/releases/tag/v8.1.3-1) |
| Die Quellen der Drittbibliotheken (x264, x265 usw.) liegen nicht im Repository; der Builder klont sie zur Bauzeit an gepinnten Commits. Upstream veröffentlicht kein vollständiges „Corresponding Source“-Bündel pro Release; GitHubs automatisches Quellarchiv umfasst nur den FFmpeg-Fork und die Builder-Skripte. | [50-x264.sh](https://raw.githubusercontent.com/jellyfin/jellyfin-ffmpeg/v8.1.3-1/builder/scripts.d/50-x264.sh) |
| Upstream-Inkonsistenz: `debian/copyright` nennt die Debian-Binaries „GPL v2+“, obwohl `debian/rules` `--enable-version3` setzt. | [debian/copyright](https://raw.githubusercontent.com/jellyfin/jellyfin-ffmpeg/v8.1.3-1/debian/copyright), [debian/rules](https://raw.githubusercontent.com/jellyfin/jellyfin-ffmpeg/v8.1.3-1/debian/rules) |

### .NET-Laufzeit

| Fakt | Quelle |
| --- | --- |
| dotnet/runtime ist MIT. Das Runtime-Pack `Microsoft.NETCore.App.Runtime.osx-arm64` 10.0.12 enthält `LICENSE.TXT` und `THIRD-PARTY-NOTICES.TXT` im Paketstamm. | [LICENSE.TXT](https://raw.githubusercontent.com/dotnet/runtime/main/LICENSE.TXT), [THIRD-PARTY-NOTICES.TXT](https://raw.githubusercontent.com/dotnet/runtime/main/THIRD-PARTY-NOTICES.TXT) |
| Die Datei `data/RuntimeList.xml` des Packs, aus der das SDK die zu kopierenden Dateien liest, führt die beiden Hinweisdateien **nicht** auf. Ein normaler self-contained Publish kopiert sie daher nicht in das Ausgabeverzeichnis (Schlussfolgerung aus dem SDK-Code; empirisch nicht in dieser Umgebung geprüft, da kein SDK vorhanden). | [ResolveRuntimePackAssets.cs](https://raw.githubusercontent.com/dotnet/sdk/main/src/Tasks/Microsoft.NET.Build.Tasks/ResolveRuntimePackAssets.cs) |

### MPL-2.0-Teile (Mutti Connect, Apple-Brücke, yamux)

| Fakt | Quelle |
| --- | --- |
| MPL 2.0 ist dateibezogen; Abschnitt 3.3 erlaubt ein „Larger Work“ unter beliebigen Bedingungen, solange die MPL-Dateien unter MPL bleiben, und erlaubt die zusätzliche Weitergabe der MPL-Dateien unter einer „Secondary License“ (GPL 2.0, LGPL 2.1, AGPL 3.0, jeweils oder später), sofern kein Exhibit-B-Hinweis „Incompatible With Secondary Licenses“ gesetzt ist. | [MPL-2.0-Text (yamux v0.1.2)](https://raw.githubusercontent.com/hashicorp/yamux/v0.1.2/LICENSE) |
| yamux v0.1.2 und die Mutti-Connect-Quellen tragen keinen Exhibit-B-Hinweis. | [session.go](https://raw.githubusercontent.com/hashicorp/yamux/master/session.go), `mutti/connect/*.go` |

### Sora (Schrift)

| Fakt | Quelle |
| --- | --- |
| Sora steht unter SIL OFL 1.1 **ohne Reserved Font Name**. Die OFL erlaubt das Bündeln mit Software, wenn Copyright-Hinweis und Lizenz mitgeführt werden; die Schrift darf nicht einzeln verkauft werden. Die OFL-FAQ erlaubt WOFF/WOFF2-Konvertierung unter Beibehaltung des Namens, wenn nur komprimiert und die Metadaten erhalten wurden. | [OFL.txt](https://raw.githubusercontent.com/sora-xor/sora-font/master/OFL.txt), [OFL-FAQ](https://raw.githubusercontent.com/silnrsi/font-charis/master/OFL-FAQ.txt) |

### kurtz und App Store

| Fakt | Quelle |
| --- | --- |
| mpv ist standardmäßig GPLv2 „or later“; ein LGPL-Build-Schalter erzeugt für sich keine LGPL-Lizenz, und gelinkte GPL-Bibliotheken (FFmpeg) bestimmen die Lizenz des Binaries mit. | [mpv Copyright](https://raw.githubusercontent.com/mpv-player/mpv/master/Copyright) |
| VLC für iOS ist doppelt lizenziert (GPLv2 oder später **und** MPLv2); das ist der einzige aus dieser Umgebung erreichbare Primärbeleg dafür, wie ein GPL-Player den App Store erreicht hat. Die FSF-Stellungnahmen von 2010, Apples EULA und VideoLANs Pressemitteilung von 2013 waren nicht abrufbar und sind nur aus Sekundärquellen bekannt. | [vlc-ios COPYING](https://raw.githubusercontent.com/videolan/vlc-ios/master/COPYING) |

## 2. Was daraus für das Mutti-Paket folgt

Keine Rechtsbewertung, sondern die Konsequenzen, die aus den Fakten folgen:

1. **Upstreams eigene Arbeitsposition** lautet: Quellcode „GPL unversioned“, ausgelieferte Binaries werden als GPL Version 3 behandelt; Plugins linken gegen GPL-3.0-only-Pakete und werden GPLv3. Ein Mutti-Paket, das Server, GPL-3.0-only-Plugin und GPLv3-FFmpeg zusammen unter **GPL Version 3** anbietet, folgt dieser Upstream-Praxis. Die Mutti-eigenen Dateien sind GPL-2.0-or-later und damit mit GPLv3 verteilbar; die MPL-Teile bleiben als eigene Dateien unter MPL-2.0 und können zusätzlich unter der Secondary License weitergegeben werden.
2. Die frühere Aussage in `THIRD-PARTY.md`, der Jellyfin-Server sei „GPL-2.0-or-later“, war durch keine Quelle gedeckt und ist korrigiert.
3. **FFmpeg-Lizenztexte** fehlten im Bundle; der Mac-Build legt jetzt `COPYING.GPLv3` und `LICENSE.md` des gepinnten Tags (per SHA-256 geprüft) in `Contents/Resources/licenses/` ab.
4. **.NET-Hinweise** (`LICENSE.TXT`, `THIRD-PARTY-NOTICES.TXT`) werden jetzt explizit aus dem Runtime-Pack in das Bundle kopiert; der Build bricht ab, wenn sie fehlen.
5. **Passende Quellen (Corresponding Source)** müssen für FFmpeg aus dem Fork-Tag, den Builder-Skripten und den gepinnten Drittbibliotheken selbst zusammengestellt oder per schriftlichem Angebot zugesagt werden; Upstream liefert kein fertiges Bündel.

## 3. Offene Punkte

1. **Außenlizenz des Pakets: entschieden.** Der Owner hat am 07.10.2026 den Vorschlag aus Punkt 2.1 übernommen: Das Gesamtpaket wird unter der GNU GPL Version 3 angeboten, Dateilizenzen bleiben erhalten ([plan.md, Abschnitt 15](plan.md#15-entscheidungsprotokoll-vom-6-oktober-2026)). Der GPLv3-Text liegt als `mutti/licenses/GPL-3.0.txt` im Repository und wird als `licenses/Mutti-Package-GPL-3.0.txt` ins Bundle kopiert; `THIRD-PARTY.md` nennt die Entscheidung. Eine anwaltliche Bestätigung vor dem ersten öffentlichen Download bleibt empfohlen, weil Upstream die GPL-Version seines Quellcodes selbst nicht belegt.
2. **FFmpeg-Quellbündel** erzeugen: Tag `v8.1.3-1` samt Builder und allen in `builder/scripts.d/` gepinnten Drittquellen, als Archiv neben dem Release oder als schriftliches Angebot.
3. **Sora-Provenienz** dokumentieren: aus welcher TTF/OTF die WOFF2-Dateien erzeugt wurden und ob die Metadaten erhalten sind (OFL-FAQ 2.2.1).
4. **kurtz im App Store**: Ohne Lösung für den GPL-mpv-Build gibt es keinen Apple-TV-Client. Mögliche Wege laut Primärbeleg: Doppellizenzierung wie bei VLC ist nur für eigenen Code möglich, nicht für mpv/FFmpeg; also Store-Build ohne GPL-Decoder oder eine andere Lizenzgrundlage. Entscheidung im kurtz-Repository, hier als Abhängigkeit geführt.
5. **Intro-Skipper-Vorgeschichte**: Teile von `Plugin.cs` tragen Copyright-Zeilen von Jellyfin-Kernentwicklern (2019/2021, Template-Herkunft); unter welcher Lizenz dieser Template-Code ursprünglich stand, wurde nicht zurückverfolgt.
6. **Empirische Prüfung** auf einem Mac mit SDK, dass der Publish-Output die .NET-Hinweise tatsächlich nicht enthält und der Build-Schritt sie korrekt einsammelt.

## 4. Gate

- Vor dem ersten öffentlichen Download: Punkt 3.2 und 3.3 erledigt, App-Info und Website nennen GPL Version 3, Bundle enthält alle Lizenztexte (Paket-GPLv3, Jellyfin, Web, Intro Skipper, FFmpeg, .NET, Sora, MPL-Komponenten); anwaltliche Bestätigung von 3.1 eingeholt.
- Vor dem Apple-TV-Release: Punkt 3.4 im kurtz-Repository entschieden.
- Der Release-Workflow (Signatur, Notarisierung) ersetzt diese Prüfung nicht.
