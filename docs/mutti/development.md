# Lokale Entwicklungsvorschau

Die Vorschau benutzt ausschließlich neue Testdaten. Keine vorhandene Jellyfin-
Datenbank übernehmen. Noch keine automatische Kopplung oder Fernverbindung.

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
Der Server bindet nur `127.0.0.1:18596`; ein belegter Port führt zu einer Meldung.
Das App-Paket wird lokal ad-hoc signiert, nicht notarisiert. Kein öffentlicher Download.

## Docker / NAS

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

Auf dem Docker-Host `http://127.0.0.1:18597/web/` öffnen. Beim Test auf einem NAS
ist bis zur sicheren Kopplung ein lokaler SSH-Tunnel nötig:
`ssh -L 18597:127.0.0.1:18597 nas`. Das ist ein Entwicklerweg und erfüllt noch
nicht die geplante einfache NAS-Einrichtung. Den Container nicht durch Ändern
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
