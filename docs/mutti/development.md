# Lokale Entwicklungsvorschau

Die Vorschau benutzt für Tests ausschließlich neue Testdaten oder den
dokumentierten Importablauf ([import.md](import.md)); nie eine fremde Jellyfin-
Datenbank direkt öffnen. QR-Kopplung und direkter Transport sind als Teststand
vorhanden ([connect.md](connect.md)). Die erste Iteration ist Mac-only; der
Docker-Abschnitt unten ist ein Entwicklerweg, keine Nutzeranleitung.

## Quellen

Beide Repositories nebeneinander klonen. `mutti/components.lock.json` enthält
den zusammenpassenden Web-Commit. Der Server behält .NET- und API-Version 12.1;
die Produktversion lautet separat `0.1.0-dev`.

## Mac

Voraussetzungen: macOS 14+, Xcode/Swift 6, .NET SDK 10.0.401, Node 26.7.0,
npm, Python 3.9+, curl. Apple Silicon wurde lokal getestet. Intel kann mit
`MUTTI_MAC_RID=osx-x64` gebaut werden und braucht eine eigene Laufzeitabnahme.

```sh
mutti/packaging/build-mac.sh
open build/macos/osx-arm64/Mutti.app
```

Bei einem abweichenden .NET-Pfad `MUTTI_DOTNET=/pfad/zu/dotnet` setzen. Nur für
bewusst lokale Änderungen erlaubt `MUTTI_ALLOW_DIRTY=1` einen nicht sauberen Baum;
dies wird im mitgelieferten Build-Nachweis vermerkt. Die App enthält Server,
Weboberfläche und per SHA-256 geprüftes FFmpeg. Keine zweite Serverinstallation
oder Terminaleingabe zum Start nötig. Ordner werden über den nativen Dialog
ausgewählt. Das Menüleistensymbol hält den Server bei geschlossenem Fenster
erreichbar; „Mutti beenden“ beendet auch den eigenen Serverprozess.

Daten liegen getrennt unter `~/Library/Application Support/Mutti Preview/`.
Jellyfin bindet nur `127.0.0.1:18596`, Einrichtung und Geräteverwaltung liegen
auf `127.0.0.1:18594` und `127.0.0.1:18595`. Nach abgeschlossener Einrichtung
lauscht zusätzlich der LAN-Vermittler für die Heimnetz-Kopplung auf TCP 18599
auf allen Schnittstellen; er trägt keine Mediendaten. Ein belegter Port führt zu
einer Meldung. Der Manager startet Jellyfin nach einem Absturz mit wachsendem
Abstand neu und beendet sich samt Server, wenn die App endet oder abstürzt.
Das App-Paket wird lokal ad-hoc signiert, nicht notarisiert. Kein öffentlicher
Download. Signierte Release-Builds: [Release-Workflow](../../mutti/packaging/macos/README.md).

## Docker / NAS (Entwicklerweg, spätere Iteration)

Seit dem 6. Oktober 2026 ist Docker/NAS kein Gate der ersten Auslieferung
([plan.md](plan.md), Abschnitt 1). Das Paket wird weiter gebaut und in CI geprüft,
damit der gemeinsame Kern nicht auseinanderläuft. Die folgenden Schritte sind
für Entwickler, nicht für Endnutzer.

Docker mit Compose v2 / BuildKit und Linux arm64 oder amd64. Die drei Verzeichnisse
vorab neu anlegen. UID/GID müssen dort Schreibrechte haben; Medien werden nur
lesbar eingebunden. Bestehende Datenbankverzeichnisse nicht wiederverwenden.

```sh
cd mutti/packaging/docker
export MUTTI_DATA=/absoluter/pfad/mutti-test/config
export MUTTI_CACHE=/absoluter/pfad/mutti-test/cache
export MUTTI_MEDIA=/absoluter/pfad/testmedien
export MUTTI_UID=1000 MUTTI_GID=1000
docker compose up --build -d
```

Auf dem Docker-Host `http://127.0.0.1:18594/` (Einrichtung und Import) und
`http://127.0.0.1:18597/web/` (Bibliothek) öffnen. Beim Test auf einem NAS ist
ein lokaler SSH-Tunnel mit denselben Portnummern nötig, weil Host und Origin
exakt geprüft werden:
`ssh -L 18594:127.0.0.1:18594 -L 18595:127.0.0.1:18595 -L 18597:127.0.0.1:18597 nas`.
Das ist ein Entwicklerweg und erfüllt nicht die geplante einfache NAS-Einrichtung. Den Container nicht durch Ändern
der Port-Bindung ins Heimnetz oder Internet öffnen. Es gibt keine Host-Netzwerk-
Freigabe, privilegierten Container oder Schreibrechte auf Medien.

Das Image enthält dieselben Mutti-Quellen. Der Linux-FFmpeg-Build stammt aus dem
per Digest festgelegten Jellyfin-Runtime-Image; die Mac-Variante ist separat gepinnt.
Hardware-Transcoding / NAS-GPU-Passthrough sind noch nicht abgenommen.

## Prüfungen

```sh
dotnet test tests/Jellyfin.Server.Tests/Jellyfin.Server.Tests.csproj --filter FullyQualifiedName~PreviewBoundary -c Release
(cd mutti/experiments/directlab && go test -v -count=1 -timeout=90s ./...)
```

Der `directlab`-Test startet zwei echte tsnet-Knoten, einen lokalen Test-Control-
Server und ausschließlich STUN. Es existiert kein DERP-Listener. Das ist ein
Ein-Rechner-Nachweis, kein WAN-Nachweis und kein deploybarer Control-Dienst.

Logs bleiben lokal und können Mediennamen enthalten. Vor Weitergabe manuell
bereinigen. Es gibt keinen automatischen Diagnoseupload. Backups/Restore,
QR-Widerruf und Plattform-Langzeittests gehören zu den offenen Release-Gates.

## Jellyfin-Übernahme

Der Importdienst `mutti/migrate` gehört zum Mac- und Docker-Paket. Er startet
Jellyfin und erst nach dessen abgeschlossenem Setup den unveränderten Connect-
Dienst. Einstieg auf Port 18594, Details und Tests unter [import.md](import.md).
Die Go-Module sind getrennt; der im kurtz-Client gepinnte Connect-Quellstand
wird durch den Import nicht geändert.
