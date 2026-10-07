# Mutti Connect – Teststand

Stand: 4. Oktober 2026. QR-Kopplung und direkter Transport sind implementiert.
Dies ist eine Entwicklungsvorschau, kein freigegebenes oder notarisiertes Release.

## Mac testen

1. Mutti öffnen und den vorhandenen Einrichtungsassistenten abschließen.
2. In der Titelleiste **Geräte koppeln** öffnen. Mit dem lokalen Besitzerzugang
   anmelden. Falls noch kein Wiedergabeprofil existiert, eines erstellen.
3. **Gerät koppeln** zeigt einen QR-Code und den kopierbaren Kopplungslink.
   Die Einladung gilt fünf Minuten und kann nur von einem Geräteschlüssel
   verwendet werden. Sie bleibt bis zur ausdrücklichen Besitzerfreigabe gesperrt.
4. Im kurtz-Testbuild unter **Verbinden → Mit Mutti koppeln** den Code mit dem
   iPhone/iPad scannen oder den Link auf Mac/Apple TV einfügen. Ein auf iOS
   gescannter `kurtz://pair`-Link öffnet auch direkt die Kopplungsansicht.
   Falls kurtz bereits angemeldet ist: über **Einstellungen → Benutzer wechseln**
   zur Konto-/Serverauswahl zurückkehren und dort die Verbindung hinzufügen.
5. Den Fingerabdruck auf beiden Seiten vergleichen, in Mutti das Gerät und sein
   Wiedergabeprofil bestätigen. kurtz meldet dieses Profil an.
6. Film starten, spulen, App neu starten, Gerät in Mutti sperren. Die Sperre muss
   laufende Übertragungen schließen und weitere Anfragen verweigern.

Ohne öffentliche Vermittlungsadresse funktioniert die Kopplung im Heimnetz.
Mutti bietet dafür einen LAN-Vermittler auf TCP 18599 an; der eigentliche
Jellyfin-Server und die Geräteverwaltung bleiben an Loopback gebunden. Eine
macOS-Abfrage zum lokalen Netzwerk muss für die Test-Apps erlaubt werden.
Eine lokale Firewall darf die direkten UDP-Pakete nicht blockieren.

**Für den Mobilfunk-/WAN-Test braucht es einen öffentlichen Vermittler.** Ein
Heimnetz-QR enthält eine private Adresse und funktioniert unterwegs nicht. Es
wurde kein öffentlicher Dienst eingerichtet oder Hosting gebucht. Trage zuerst
unter **Fernzugriff** die HTTPS-Adresse und den STUN-Endpunkt des Testdiensts ein
und kopple die Geräte danach neu. Bereits bestehende Geräte behalten ihren
Schlüssel, ihre gespeicherte Vermittlungsadresse ändert sich dadurch nicht.

## Vermittler auf einem vorhandenen öffentlichen Server

Der Ordner `mutti/packaging/rendezvous/` enthält Dockerfile, Compose-Datei und
Beispiel für einen vorhandenen Caddy-Reverse-Proxy. Der Dienst braucht einen
öffentlichen DNS-Namen mit gültigem HTTPS-Zertifikat und erreichbares UDP 3478.

```sh
docker compose -f mutti/packaging/rendezvous/compose.yaml up -d --build
```

Caddy auf dem Host leitet HTTPS an `127.0.0.1:18600` weiter. In Mutti etwa
`https://connect.example.org` und `stun:connect.example.org:3478` eintragen.
Ein reiner HTTP-Tunnel kann die Signalisierung transportieren, ersetzt aber den
separat erreichbaren STUN-Endpunkt nicht. Keine TURN-/DERP-Konfiguration verwenden.

Der Vermittler ist accountlos und hat keine Mediendaten- oder Proxy-Endpunkte.
Er sieht IP-Adressen und SDP/ICE-Verbindungsmetadaten, nicht die TLS-geschützte
Gerätefreigabe oder Bibliothek. Mailboxen und Verbindungsangebote sind flüchtig,
begrenzt und laufen spätestens nach einer Minute ab. Der Betreiber muss auch
beim vorgeschalteten Proxy Zugriffslogs deaktivieren, Updates einspielen und
Missbrauch/Verfügbarkeit betreuen. Die eingebauten Mengenbegrenzungen sind für
eine kleine Testinstallation, nicht für einen großen öffentlichen Dienst ausgelegt.

## Docker/NAS

Das normale Mutti-Image enthält denselben Verbindungsdienst. Es benötigt keine
TUN-Geräte, keine Netzwerk-Erweiterung und keine privilegierten Containerrechte.

Zusätzlich zu Daten-/Medienpfaden im bestehenden Compose-Setup setzen:

```sh
MUTTI_SIGNAL_URL=https://connect.example.org
MUTTI_STUN_URL=stun:connect.example.org:3478
```

Der UDP-Bereich 40000–40031 wird vom Container veröffentlicht. Das ist keine
Aufforderung, Jellyfin-HTTP im Router freizugeben. Für reine LAN-Kopplung statt
öffentlichem Vermittler `MUTTI_LAN_BIND` auf die LAN-IP des NAS und
`MUTTI_LAN_ORIGIN=http://NAS-LAN-IP:18599` setzen; standardmäßig ist die
Veröffentlichung des LAN-Vermittlers vorsichtig auf Loopback begrenzt.

Die Besitzerverwaltung bleibt lokal. Für die Testversion vom eigenen Rechner,
mit denselben Portnummern wie im Container, weil Host und Origin exakt geprüft werden:

```sh
ssh -L 18594:127.0.0.1:18594 -L 18595:127.0.0.1:18595 -L 18597:127.0.0.1:18597 user@nas
```

Einrichtung und Import unter `http://127.0.0.1:18594`, Bibliothek unter
`http://127.0.0.1:18597`, Geräte unter `http://127.0.0.1:18595`. Docker/NAS ist
seit dem 6. Oktober 2026 ein Entwicklerweg und kein Gate der ersten Auslieferung. Ein komfortabler, sicherer NAS-Besitzerzugang von einem
zweiten Gerät bleibt offen. Keine ungeschützte Admin-Oberfläche ins LAN stellen.

## Protokoll und Grenzen

- Pion WebRTC 4.2.22 mit UDP-ICE, Host-/STUN-Kandidaten, zuverlässigem DataChannel.
  Kein TURN-Server konfigurierbar; Relay-Kandidaten werden abgewiesen.
- Über dem Datenkanal läuft zusätzlich Standard-TLS 1.3. kurtz prüft den im QR
  enthaltenen SHA-256-SPKI-Fingerabdruck. Mutti verlangt einen durch TLS bewiesenen
  Geräteschlüssel. Ein kompromittierter Vermittler kann Verbindungen stören,
  aber ohne die Identitätsschlüssel keinen freigegebenen Mediendatenweg übernehmen.
- Einladung und Besitzerfreigabe sind von Jellyfin Quick Connect getrennt.
  Quick Connect wird erst nach der Besitzerfreigabe serverseitig für das gewählte
  nicht administrative Profil verwendet. Sein echtes Bearer-Token bleibt in
  Muttis privatem Zustand und wird nie an den Client herausgegeben.
- Jeder Medieneingang prüft weiterhin die aktuellen Profilrechte. Eine spätere
  Administratorbeförderung macht das Gerät nicht zum Administrator.
- Ein fester lokaler Gateway in kurtz führt API, Bilder, Range, HLS und WebSocket
  durch dieselbe authentifizierte Verbindung. Keine beliebigen Tunnelziele;
  Browser-Origin, falscher Host, CONNECT und fremde Redirects werden abgewiesen.
- Schlüssel auf Apple-Geräten liegen im nicht synchronisierten Keychain mit
  `ThisDeviceOnly`. Mutti speichert Identität und Geräte in einer privaten
  Datei, atomisch und mit Modus 0600. Keine Passwörter/QR-Geheimnisse in Logs.
- Widerruf wird zuerst gespeichert und schließt bestehende Gerätesitzungen.
  Neue Anfragen werden abgewiesen. Ein verlorenes Gerät wird gelöscht und neu
  gekoppelt; Geräte-/Server-Schlüsselwiederherstellung ist noch kein Produktablauf.
- Neue HTTP-Anfragen bauen nach einem Verbindungsabbruch die Verbindung erneut
  auf. Ein bereits abgebrochener Player-Stream muss gegebenenfalls neu gestartet
  werden; nahtlose Wiedergabe über Netzwechsel ist noch nicht abgenommen.

ICE-/TLS-Aufbau hat ein Zeitlimit. Symmetrisches NAT, gesperrtes UDP, bestimmte
Gast-/Firmen-/Mobilfunknetze können den direkten Weg verhindern. Kein heimlicher
Relay- oder unverschlüsselter Ersatzweg. Erfolgsquoten werden erst mit echten
getrennten Anschlüssen ermittelt. Day-2-Relay-Marktprüfung bleibt separat.

## Entwickeln und prüfen

```sh
cd mutti/connect
go test -race ./...
go vet ./...
# Im Server-Repository, benötigt bereits gebaute Mac-Server-/FFmpeg-Dateien:
python3 mutti/tests/connect-smoke.py
# Nach docker build -t mutti:connect-preview ...:
python3 mutti/tests/connect-docker-smoke.py
```

Der echte Server-Test darf nur eine frische synthetische Instanz auf dem
reservierten Loopback-Port 18598 verwenden und verweigert vorhandene Daten.
Unit-/Integrationstests begrenzen ICE auf Loopback. Für gezielte Netztests kann
`MUTTI_ICE_INTERFACES` eine kommaseparierte Schnittstellenliste vorgeben; im
normalen Paket wird diese Einschränkung nicht gesetzt.

Im kurtz-Repository:

```sh
bash Tools/MuttiConnect/build.sh catalyst
bash Tools/MuttiConnect/build.sh ios
bash Tools/MuttiConnect/build.sh tvos
# Bei Simulator-Builds zusätzlich ios-simulator bzw. tvos-simulator bauen.
```

Die Bibliothek stammt standardmäßig aus `../mutti/mutti/connect`. Der Build
schreibt eine ignorierte `XcodeConfig/MuttiConnect.local.xcconfig` und bindet den
Transport nur in entsprechend vorbereitete Testbuilds ein. Normale Jellyfin-
Verbindungen bleiben erhalten. Für Intel-Slices als zweites Argument `x86_64`
angeben. Mac über `kurtz.xcworkspace` bauen, damit die vorhandenen Catalyst-
Paketkorrekturen verwendet werden. Kein Produktionsupload durch diese Befehle.

Noch abzunehmen: WAN-/CGNAT-/IPv6-Matrix, reale NAS-Hardware, reale iOS-/tvOS-
Geräte, längere Wiedergabe und HLS-Varianten, Netzwechsel, Betrieb des öffentlichen
Vermittlers, englische Texte, vollständige Barrierefreiheit, signierte Verteilung
und unabhängige Sicherheitsprüfung. Lokal bestandene Tests ersetzen das nicht.
