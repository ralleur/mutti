# Umsetzungsstand

Stand: 5. Oktober 2026. **Lokale Entwicklungsvorschau; der Gesamtplan ist nicht abgeschlossen.**

Die Forks `ralleur/mutti` und `ralleur/mutti-web` behalten ihre vollständige
Jellyfin-Historie. Die Produktbranches beginnen beim zusammenpassenden stabilen
Stand v12.1. Die Umsetzung liegt zunächst auf `codex/mutti-foundation`; `main`
ist die Review-Basis. Der Web-Commit ist im Komponentenmanifest festgelegt.

| Etappe | Ergebnis | Noch offen |
| --- | --- | --- |
| M0 | Unveränderte Mac-Server- und Web-Referenz gebaut. Zwei echte tsnet-Knoten übertragen lokal verschlüsselt Daten mit ausschließlich STUN; kein DERP-Server läuft. | Getrennte Internetanschlüsse, gesperrtes UDP, CGNAT/IPv6, Netzwechsel; verbindliche Transport-/Control-Entscheidung. |
| M1 | Eigene Forks, Komponenten-Pins, Mac- und Docker-Builds, Entwicklungsanleitung, Sicherheitsregeln und CI implementiert. Lokale API-/Videodatenstrecke auf beiden Paketen bestanden. | Vollständiges Quell-/Lizenzinventar für Distribution, weitere Architekturen und vollständige Wiedergabeabnahme. |
| M2 | Eigene Mutti-Vektormarke, Sora, kurtz-Farben, Web-Assistent; native Mac-Hülle mit Serverstart, Status, Ordnerdialog-Brücke und getrennten Daten; nicht privilegiertes Docker-Paket mit schreibgeschützten Medien. | Native Ordnerauswahl durch alle Dialogschritte, Screenreader/Hellmodus vollständig, echte NAS-Installation; einfache sichere Verwaltung von einem zweiten Gerät. Der derzeitige NAS-SSH-Tunnel ist nur ein Entwicklerweg. |
| M2b | Automatisierter Jellyfin-12.1-Import: Erstwahl, lokale Erkennung, Admin-Anmeldung, interne Online-Sicherung, isolierte Wiederherstellung, Daten-/Dateiprüfung und atomarer Wechsel. Exporthelfer mit einmaligem Transferzugriff vorhanden. | Owner-Test mit echter Bibliothek; große Datenmengen, reale NAS-Mounts, weitere Versionen, externe Plugins/Logins. Automatische Helferinstallation/-entfernung und manueller Archiv-Ausweichweg offen. Siehe [import.md](import.md). |
| M3 | QR-Einladung, TLS-Geräteidentität, bestätigte Profilfreigabe, Keychain-Integration und laufender Widerruf implementiert und lokal geprüft. | Wiederherstellung, reale Geräte und vollständige Ablauf-/Bedienabnahme. |
| M4 | Direkter verschlüsselter Transport und Vermittlungsdienst als Teststand implementiert; siehe unten. Kein Relay. | Öffentlicher Testbetrieb, WAN-Matrix und Wiedergabe bei Netzwechseln. |
| M5 | Lokale Sicherheits- und Integrationstests vorhanden. | Backup/Restore, Upgrade, Langzeittests, reale iOS-/tvOS-Geräte, NAS und zwei echte Anschlüsse. |
| M6 | Lokale Mac-App und Docker-Image verfügbar. | Gemeinsame vollständige Abnahme, Developer-ID/Notarisierung, Quellpakete und freigegebenes Release. |

## Tatsächlich geprüft

- .NET 10.0.401, Node 26.7.0, Xcode 27.0, Go 1.27.1; Linux arm64 im lokalen Docker.
- Unveränderte Jellyfin-v12.1-Referenz für den Mac-Server und Web gebaut.
- Mutti-Server: zwölf Tests gegen fremde Hosts/Origins, gefälschte Forwarded-Header,
  nicht lokale Zugriffe und ungültige Preview-Konfiguration bestanden.
- Mutti Web: Produktionsbuild und TypeScript; ESLint für geänderte Abläufe,
  Stylelint für geänderte Styles. Web-CI des gepinnten Stands erfolgreich.
  Auch die eigenen Server-, Mac- und Linux-amd64-Container-CI-Prüfungen bestanden.
- Regionale Sprachauswahl wurde nach einem echten WKWebView-Sichttest korrigiert
  und gegen `de-DE`, `de_AT`, `en-gb` und ungültige Sprachcodes getestet.
- Mac und Docker: jeweils frische Daten, Besitzerzugang, nicht administratives
  Wiedergabeprofil, synthetisches 12-Sekunden-Video importiert, Quick Connect erst
  nach Besitzerfreigabe, Admin-Endpunkt verweigert, HTTP-Range `206` bytegenau
  geprüft, nach Geräteentfernung weitere authentifizierte Anfragen verweigert.
- Mac-App tatsächlich gestartet, Assistent sichtbar; App-Beenden entfernt den
  lokalen Server-Listener. Lokale Ad-hoc-Signatur verifiziert. Keine Notarisierung.
  Der finale Mac-Build stammt aus sauberen Server-/Web-Commits; die Sprachvorgabe
  wurde erneut im echten Fenster geprüft. Docker-Neustart erhält Serveridentität
  und abgeschlossene Einrichtung; der temporäre Testcontainer wurde entfernt.
- `directlab`: echter Userspace-WireGuard-Datenweg und Disco-Endpunkt, weder DERP
  noch Peer-Relay. Ein Rechner; keine Aussage über NAT-Erfolgsquoten im Internet.

Die Testberichte unter `docs/mutti/evidence/` enthalten keine Testpasswörter,
Zugriffstokens, realen Medien oder Benutzerkonten. Vollständige Build-/Serverlogs
bleiben lokal unter `build/` bzw. im temporären Testverzeichnis.

## QR-Kopplung und direkter Transport

Auf den Folgeauftrag „entwickle weiter inkl qr kopplung und direkter fernzugriff
 dann teste ich“ wurde M3/M4 weiter implementiert; der reale Netztest blockiert
 die Implementierung nicht mehr. Details und Testanleitung: [connect.md](connect.md).

Der neue gemeinsame Go-Dienst enthält accountlose Signalisierung, ausschließlich
 direkten Pion-Datenkanal, TLS-1.3-Identitätsbindung, kurzlebige Einladungen,
 Besitzerfreigabe pro Geräteschlüssel, Profilbindung und laufenden Widerruf.
 Mutti Mac und Docker starten ihn; kurtz erhält Keychain-Speicherung, QR-Scanner,
 Linkannahme und einen gemeinsamen lokalen Gateway für sämtliche Clientwege.

Lokal geprüft: Race-Detector und statische Go-Analyse; echte Mutti-Instanz mit
 synthetischem Video, Gerätefreigabe, nicht administrativem Profil, Range-206-
 Bytes, Jellyfin-WebSocket und Sperre nach erneutem Verbindungsaufbau. Die
 Tests verwenden Loopback-ICE, keinen WAN-Nachweis. Derselbe Ablauf besteht auch
 im tatsächlichen Docker-Paket über dessen Geräteverwaltung. Mac-, iOS- und
 tvOS-App-Builds mit eingebettetem Transport sind erfolgreich. Builddetails stehen in der
 Testanleitung. Physische Geräte und getrennte Anschlüsse prüft der Owner.

Ein öffentlich erreichbarer HTTPS-/STUN-Vermittler ist paketiert, aber nicht
 betrieben. Ohne dessen Adresse ist der Test auf das Heimnetz begrenzt. Es wurde
 kein Hosting gebucht, kein Cloudflare-Dienst angelegt und kein Relay aktiviert.
 Day 2 bleibt eine spätere neue Marktprüfung.

## Rückmeldung nach dem ersten Nutzertest (5. Oktober)

Die Mac-Hülle unterscheidet nun Serverbereitschaft und abgeschlossene Einrichtung.
Geräte-Kopplung und Fernzugriff werden erst nach Jellyfins bestätigtem Setup-Abschluss
angeboten; vorher startet auch der Kopplungsdienst nicht. Ein Neustart prüft den
Zustand erneut. Die Importanforderung MK-005 ist in Foundation M2b übernommen;
der qualifizierte Import für Jellyfin 12.1 ist jetzt als Teststand vorhanden ([Testanleitung](import.md)).

Die anschließende Owner-Präzisierung setzt einen möglichst automatischen
Ein-Klick-Import als Produktziel: lokal Sicherung intern anstoßen und direkt
lesen, remote einen temporären Umzugshelfer qualifizieren. Das ersetzt die offene
Wahl eines primär manuellen Archivablaufs. Der lokale Normalfall ist umgesetzt;
auf entfernten Servern ist die einmalige manuelle Helferinstallation noch nötig.

## Import-Teststand vom 5. Oktober

Mac arm64 und Linux arm64: vollständige synthetische Übernahme einschließlich
User-IDs/Passwörtern/Rechten, Bibliothek, Playlist, Favoriten, Wiedergabe/Resume
und erhaltenem Quellserver bestanden. Go-Race-Detector und statische Analyse,
Archivpfad-/Unvollständigkeitsprüfungen, Weiterleitungs- und CSRF-/Origin-Schutz
getestet. Exporthelfer gebaut und authentifizierter Transfer geprüft.

Mac-App gebaut und im echten WKWebView geprüft; lokaler Jellyfin wurde automatisch
erkannt. Die tatsächliche Benutzerbibliothek wurde für die Prüfung nicht importiert.
Details und bewusste Grenzen stehen in [import.md](import.md). Kein Release.

Finale Paketprüfung: Docker-Entrypoint mit ausschließlich localhost-veröffentlichten
Verwaltungsports, Browser-Einstieg, Host-/Origin-Abweisung und gesperrter Kopplung
vor Setup bestanden. Profilbild, Anzeigeeinstellungen und Neustartpersistenz
bestehen auf Mac und Linux. Bestehende .NET-Grenztests: 12/12 bestanden.
Der erste native Testbuild aus sauberem Quellstand `755b85c5d0` wurde lokal
signiert und für den Owner geöffnet; der nachfolgende korrigierte Stand steht unten.

### Korrektur nach dem ersten Owner-Test

Der bereits abgeschlossene Preview-Assistent löste eine zusätzliche Anmeldung
als „Mutti-Besitzer“ aus, obwohl beim Jellyfin-Umzug kein weiteres Konto nötig
sein soll. Die Mac-App bestätigt einen Wechsel jetzt nativ über ihren privaten
Prozesszugang; der Anwender gibt nur den bestehenden Jellyfin-Administrator ein.
Browser und Docker prüfen weiterhin den Administrator einer vorhandenen
Zielbibliothek. Quellserver und bisherige Zieldaten bleiben erhalten.

Der vollständige synthetische Mac-Import mit bereits eingerichteter Zielinstanz
und ohne Kenntnis ihres Passworts besteht einschließlich Datenprüfung und
Neustart. Go-Race-Tests, Autorisierungsgrenzen und drei Swift-Tests bestehen.

Aktuelles Testpaket: sauberer Quellstand `5347f116c7`, vollständiger Mac-Neubau
mit verifizierter lokaler Signatur. `build/connect-preview/Mutti.app` ist wieder
geöffnet. Im WKWebView erscheinen ausschließlich die Jellyfin-Zugangsfelder;
der native Wechsel-Dialog und sein Abbruch wurden geprüft. Danach bleibt der
Manager im Zustand `idle` auf dem bisherigen Datenordner. Die echte Bibliothek
wurde nicht importiert. Das neu gebaute Docker-Paket besteht den Entrypoint-Test
inklusive verweigerter nativer Berechtigung für Browseranfragen.

### Intro Skipper als Standard (MK-006)

Intro Skipper 12.0.4.0 gehört zum Mac- und Docker-Paket und wird bei
neuer Einrichtung sowie Import automatisch installiert. Vorhandene Daten dieser
Version werden ohne Plugin-Rückfrage über konsistente SQLite-Snapshots übernommen:
Konfiguration, Ausschlüsse, Segmente und Analysecache. Die Original-DLL wird aus
dem offiziellen Release mit SHA-256-Prüfung paketiert, einschließlich passendem
Quellarchiv und GPL-Lizenz. Der Exporthelfer 0.1.1.0 enthält dieselben Zusatzdaten.

Mac- und Linux-Synthetik: vollständiger Umzug mit vorhandenen Einstellungen und
Sprungmarken, erweiterter Export samt Einmaltickets sowie Neustart bestanden.
Quelle ohne Plugin ebenfalls auf Mac und Linux geprüft. Die Prüfung fordert den
FFmpeg-Funktionsstatus „okay“ des Plugins, nicht nur seinen Installationsstatus.
14 .NET-Tests einschließlich konsistenter WAL-Sicherung, Quellen-Erhalt und
Pfadgrenzen, drei Swift-Tests sowie Go-Race-Tests/statische Analyse bestanden.
Der Restore setzt Hardwarebeschleunigung korrekt auf `none` und bewahrt den
Paket-FFmpeg-Pfad beim internen Neustart. Andere aktive Plugins oder
unqualifizierte Intro-Skipper-Versionen werden weiterhin erkannt.

Finale Pakete aus sauberem Quellstand `2e97113d81`: Mac vollständig gebaut und
lokale Signatur verifiziert; Docker-Entrypoint inklusive automatisch installiertem
Plugin und bisherigen Netzwerk-/Setup-Grenzen bestanden. Der geöffnete Mac-Build
unter `build/connect-preview/Mutti.app` lädt Intro Skipper 12.0.4.0 aktiv und ohne
Chromaprint-Startfehler. Importdialog und lokale Servererkennung geprüft; keine
zusätzlichen Zielkonto-Felder. Der Manager bleibt vor dem Owner-Import auf dem
ursprünglichen Datenordner im Zustand `idle`. Die echte Jellyfin-Bibliothek wurde
für diese Abnahme nicht importiert.

### Importfortschritt und blockierte Quellsicherung (MK-005)

Nach dem Owner-Test zeigt der Import sieben Arbeitsschritte, Gesamt-/Schrittdauer,
beobachtete Sicherungsgrößen und die Zeit seit messbarer Änderung. Nach 90 Sekunden
erscheint ein qualifizierter Wartehinweis. Verbindungsabbruch, 30-Minuten-API-Limit,
45-Minuten-Gesamtlimit und Benutzerabbruch werden unterschieden. Keine geschätzte
Prozentzahl oder Erfolgsmeldung aus bloßen Statusabfragen.

Die reale Quelle blieb im Sperrmodus `Pessimistic` in der Datenbanksicherung
stehen. Mutti fragt diesen Modus jetzt vor dem Backup ab und startet dafür keine
weitere Sicherung. Die vorhandene Quelle wurde für die Diagnose ausschließlich
lesend geprüft; keine Konfiguration geändert und kein Neustart ausgelöst.
Der bisherige Mutti-Datenstand blieb aktiv. Details: [import.md](import.md).

Synthetischer vollständiger Mac-Import, Go-Race-Tests und statische Analyse sowie
Sichtprüfung für Wartehinweis und unterbrochene Verbindung bestanden.

Pakete aus sauberem Commit `2124e7f499`: vollständiger Mac-Build mit verifizierter
Ad-hoc-Signatur sowie Docker-Neubau bestanden. Vollständiger synthetischer
Linux-Import mit Fortschrittsdaten, Intro Skipper, Datenprüfung und Neustart
bestanden; Docker-Entrypoint-/Host-/Origin-/Setup-Grenzen ebenfalls. Die neue
Mac-App unter `build/connect-preview/Mutti.app` wurde erst nach dem Zeitlimit
des alten Imports ausgetauscht und gestartet. Ihre API liefert die neuen
Fortschrittsdaten; der bisherige aktive Datenordner bleibt erhalten. Der
Quellserver benötigt weiterhin die bewusste Sperrmodus-Korrektur und einen Neustart.


### Automatische Quellvorbereitung (MK-005)

Der Import übernimmt jetzt Sicherung und Korrektur des problematischen
Jellyfin-SQLite-Sperrmodus sowie den begleiteten Neustart. Ein Hinweis am
Importknopf und in der bestehenden nativen Wechselbestätigung ersetzt die
manuelle Einstellungssuche. Browser/Docker verwenden ausschließlich den
Quelladministrator; die Mac-App kann zusätzlich einen eindeutig zugeordneten
LaunchAgent des angemeldeten Benutzers auch bei blockierter Anmeldung neu starten.
Serveridentität und Bereitschaft werden vor dem Sicherungsauftrag erneut geprüft.
Originalkonfiguration und ein noch ausstehender Neustart bleiben privat gespeichert.

Synthetisch bestanden: vollständiger Mac-Import nach API-Umstellung/Neustart sowie
nach absichtlich blockierter Datenbanksicherung und Wiederanlauf über launchd.
Der zweite Lauf erhält Benutzer, Playlist, Favoriten, Wiedergabestand, Intro Skipper
und Neustartpersistenz. Go-Race-Tests/statische Analyse prüfen Administrator- und
Dateigrenzen, Konfigurationserhalt, fremde Identität, Wiederaufnahme und fehlenden
Neustartnachweis. Drei Swift-Tests bestanden. Die Erkennung des vorhandenen
Owner-Dienstes wurde ausschließlich lesend geprüft; keine echte Quellkonfiguration
geändert und kein echter Import gestartet.

Paketabnahme: Docker arm64 aus `27a1563a82` besteht den vollständigen synthetischen
Import mit automatischer API-Umstellung und Neustart sowie den Entrypoint-/Host-/
Origin-/Setup-Test. Mac arm64 aus `8c08c7c4d7` enthält denselben Importkern und die
zusätzliche Korrektur einer falschen Portbelegt-Meldung beim schnellen Wiederöffnen
(`TIME_WAIT`). Vollständiger Build aus sauberen Quellen, Ad-hoc-Signatur und drei
Swift-Tests bestanden; aktualisierte App unter `build/connect-preview/Mutti.app`
bereitgestellt. Der ursprüngliche Quelldienst wurde durch den Agenten nicht neu
gestartet; die tatsächliche Migration startet der Owner über den Importdialog.

### Interne Sammlungen bei der Importprüfung (MK-005)

Der Owner-Import erreichte nach mehreren Minuten die Bibliotheksprüfung und
stoppte vor Aktivierung. Die ausschließlich lesende Diagnose zeigte erhaltene
Bibliotheks-IDs und externe Medienpfade. Die interne Sammlung wurde korrekt in
das neue Datenverzeichnis übernommen, aber mit ihrem alten, von Jellyfin in der
API aufgelösten Pfad verglichen. Archivaufbereitung und Bibliotheksprüfung teilen
jetzt dieselbe Pfadzuordnung. IDs, Namen und vollständige Ordnerlisten bleiben
verbindlich; echte Abweichungen benennen die betroffene Bibliothek und Fehlerart.

Eine künstliche Sammlung reproduziert vor der Korrektur dieselbe Fehlermeldung.
Mit der Korrektur besteht der vollständige Mac-Import samt Sammlung/Zuordnung,
Intro Skipper, Benutzerrechten, Wiedergabestand und Neustartpersistenz. Go-Race-
Tests und statische Prüfung bestanden. Die echte Quelle und der fehlgeschlagene
Importordner wurden für die Diagnose nicht verändert.

Paketabnahme aus sauberem Commit `cf343d5353`: Mac vollständig gebaut und lokal
signiert, Docker arm64 neu gebaut. Vollständiger synthetischer Linux-Import mit
Sammlung, Intro Skipper, erhaltenen Daten und Neustart bestanden; Docker-Entrypoint
und Zugriffsgrenzen ebenfalls. Die aktualisierte App unter
`build/connect-preview/Mutti.app` ist geöffnet, der Importdialog erkennt die lokale
Quelle. Der Manager steht auf `idle` mit dem bisherigen aktiven Datenordner.
Der nächste echte Import bleibt beim Owner; keine echte Bibliothek wurde durch
den Agenten importiert oder aktiviert.


### Kurt begleitet den Import (MK-010)

Die beauftragten sieben Animationen ersetzen den Spinner: Schlafen, Po-Rutschen,
Gehen, Laufen, Leckerli, Kotzen, Häufchen. Nach der Zugangsprüfung läuft einmal
Aufwachen/Aufstehen; die echte Verarbeitung wartet nie darauf. Bewegungen wenden
an den Bühnenrändern, Hinterlassenschaften werden je Schleife neu dargestellt.
Die übrigen Fortschrittsdaten bleiben sichtbar. Pause, reduzierte Bewegung,
verlorene Statusverbindung, ausgeblendete Seite und Beendigung halten die Figur an.

Das lokale Paket enthält die unveränderten benötigten Zeichnungen aus Hausers
Kurt-Fassung 11: 127 einzigartige Bildzellen auf zwei verlustfreien WebP-Atlanten,
insgesamt rund 3 MiB einschließlich Effekten und Herkunftsnachweisen. Keine neuen
Bilder erzeugt, keine Hauser-Anwendungslogik übernommen, kein externer Abruf.
Die öffentliche Lizenzierung der privaten Zeichnungen wird damit nicht verändert.

Sieben fokussierte Animationstests und die Go-Importtests bestanden. Die echte
Weboberfläche wurde mit synthetischen Zuständen angesehen: Schlafen, Übergang zum
Sichern, Kotze/Häufchen, Pause und unverändertes Bild, Verbindungsabbruch und Fehler
mit gestoppter Animation. Der Importkern und echte Benutzerdaten wurden dafür
nicht verändert. Mac-/Docker-Paketabnahme folgt für diesen Stand unten.

Paketabnahme aus sauberem Commit `00797474c3`: Mac arm64 vollständig gebaut,
Ad-hoc-Signatur verifiziert, laufende Test-App ausgetauscht und geöffnet. Der
Importdialog erkennt die lokale Quelle; der Manager ist bereit auf dem bisherigen
Datenordner. Das ausgelieferte Kurt-Manifest und ein Atlas wurden über die echte
Mac-API mit dem Quellpaket verglichen. Docker arm64 gebaut; Entrypoint-Test mit
sämtlichen benötigten Clip-/Bildressourcen, Herkunftsgrenzen und gesperrter
Kopplung vor Setup bestanden. Der echte Import wurde nicht vom Agenten gestartet.
