# Lizenzprüfung vor der ersten Distribution

Stand: 6. Oktober 2026. Dieses Dokument sammelt die im Review festgestellten
Lizenzfakten und offenen Fragen. Es ist keine Rechtsberatung und trifft keine
Lizenzentscheidung. Vor dem ersten öffentlichen Download muss jede offene Frage
mit Nachweis beantwortet sein; bis dahin gilt sie als Release-Blocker.

## Festgestellte Fakten

| Komponente | Befund im Repository | Quelle |
| --- | --- | --- |
| Jellyfin Server | Die Datei `LICENSE` enthält den Text der GNU GPL **Version 2**. Die NuGet-Projektdateien `MediaBrowser.Common`, `MediaBrowser.Controller`, `MediaBrowser.Model`, `Jellyfin.Extensions`, `Jellyfin.Database.Implementations` und `Jellyfin.MediaEncoding.Keyframes` deklarieren `PackageLicenseExpression` **GPL-3.0-only**. Upstream kommuniziert „GPL 2.0“. | `LICENSE`, `*.csproj` |
| Jellyfin Web | GPL-2.0-or-later laut Upstream-LICENSE. | `mutti-web/LICENSE` |
| Eigener Mutti-Code | `GPL-2.0-or-later` in SPDX-Kopfzeilen (`mutti/migrate`, `mutti/apps/macos`, `mutti/intro-skipper`); Connect-Transport und Apple-Brücke `MPL-2.0`. | `mutti/THIRD-PARTY.md`, Kopfzeilen |
| Intro Skipper | Offizielles Plugin 12.0.4.0, `GPL-3.0-only`, unverändert gebündelt und im Jellyfin-Prozess geladen. | `mutti/components.lock.json` |
| Jellyfin FFmpeg | GPL-Build; Mac 8.1.3-1 per Hash, Docker 8.1.2 aus dem Upstream-Image. Passende Quellen, Build-Konfiguration und Bibliotheksquellen sind noch nicht gebündelt. | `components.lock.json`, `THIRD-PARTY.md` |
| .NET-Laufzeit | MIT mit Third-Party-Notices im Publish-Output. | Publish-Output |
| Sora | SIL Open Font License 1.1, gebündelt. | `mutti/design/assets/Sora-OFL.txt` |
| Go-Abhängigkeiten (Connect) | Pion (MIT), yamux (MPL-2.0), go-qrcode (MIT), x/time (BSD-3). | `mutti/connect/go.mod` |
| kurtz (Client) | Quelle MPL-2.0; der ausgelieferte Mac-Build ist durch den GPL-mpv-Build GPL-3.0-or-later. Der kurtz-Release-Plan nennt die App-Store-Verträglichkeit des GPL-Decoders als ungelöst. | `ralleur/kurtz`, `docs/release/apple-release-plan.md` |

## Offene Fragen

1. **Welche GPL-Version gilt für den Jellyfin-Server tatsächlich?** Text und
   NuGet-Deklaration widersprechen sich. Davon hängt ab, ob ein Produkt aus
   Server (GPL-2.0-only oder -or-later?) und Intro Skipper (GPL-3.0-only) im
   selben Prozess als Kombination verteilt werden darf. Bei GPL-2.0-or-later ist
   die Kombination unter GPL-3.0 verteilbar; bei GPL-2.0-only nicht ohne
   weiteres. Nachweis einholen (Upstream-Aussage, Dateikopfzeilen, Historie).
2. **Unter welcher Lizenz wird das Mutti-Gesamtpaket angeboten?** Die
   Kopfzeilen sagen GPL-2.0-or-later, das Paket enthält GPL-3.0-only-Teile.
   Die Außen-Lizenz des Pakets, der THIRD-PARTY-Text und die App-Info müssen
   übereinstimmen.
3. **FFmpeg-Quellbündel:** Vor der Distribution den exakt passenden Quellstand
   samt Build-Rezept bereitstellen oder ein schriftliches Angebot beilegen.
   Mac und Docker sollten dieselbe Version verwenden.
4. **kurtz im App Store:** Ohne App-Store-Freigabe gibt es keinen Apple-TV-
   Client und damit keinen zentralen Teil der Vision. Die Entscheidung über den
   GPL-Decoder (ersetzen, nachlizenzieren, Store-Build ohne mpv) gehört in den
   kurtz-Release-Plan und wird hier als Abhängigkeit geführt.
5. **MPL-2.0 neben GPL:** Die MPL-Teile (`mutti/connect`, Apple-Brücke, yamux)
   sind mit GPL kompatibel, solange sie nicht als „Incompatible With Secondary
   Licenses“ gekennzeichnet sind. Beim Bündeln die Dateikopfzeilen und
   Hinweise prüfen und die MPL-Quellen zugänglich halten.
6. **Marken:** Jellyfin-Markenhinweise bleiben erhalten; der Name „Mutti“ ist
   als Produktname gesetzt, eine Markenrecherche ist nicht Teil dieses
   Dokuments.

## Gate

- Vor dem ersten öffentlichen Download: Fragen 1 bis 3 mit Nachweis beantwortet,
  `THIRD-PARTY.md` und App-Info angepasst, Quellbündel verfügbar.
- Vor dem Apple-TV-Release: Frage 4 im kurtz-Repository entschieden.
- Der Release-Workflow (Signatur, Notarisierung) ersetzt diese Prüfung nicht.
