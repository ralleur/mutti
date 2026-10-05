# Jellyfin nach Mutti übernehmen

Stand: 5. Oktober 2026 · lokale Entwicklungsvorschau für **Jellyfin 12.1**.

## Testen

Mac-App: `build/macos/osx-arm64/Mutti.app` (Apple Silicon). Startet mit dem
bisherigen privaten Mutti-Preview-Datenordner. Bei einer neuen Installation
erscheint zuerst **Neu einrichten / Aus Jellyfin übernehmen**. In einer bereits
eingerichteten Mutti-App führt **Jellyfin übernehmen** oben direkt zum Import.

1. Den erkannten Jellyfin-Server wählen oder seine Adresse eingeben und als
   Administrator anmelden. In der Mac-App genügt dieser vorhandene Jellyfin-Zugang.
   Bei einer schon eingerichteten Mutti-Instanz den Wechsel im nativen Dialog
   bestätigen; deren Daten bleiben erhalten. Kein zusätzliches Mutti-Konto anlegen.
2. **Jellyfin übernehmen** starten. Währenddessen Wiedergabe und Änderungen auf
   Jellyfin pausieren. Nach der Abschlussprüfung mit dem bestehenden Jellyfin-
   Zugang anmelden und Geräte neu koppeln.

Keine manuelle ZIP-Erstellung, Dateisuche oder neue Benutzeranlage im lokalen
Normalfall. Die Quelldaten und Medien werden nicht verschoben. Jellyfin bleibt
installiert und läuft weiter; nach dem Wechsel bitte Mutti verwenden. Es gibt
keine laufende Synchronisierung zwischen beiden Servern.

Docker/NAS verwendet denselben Dienst und Ablauf. Der neue Einstieg liegt auf
`http://127.0.0.1:18594`, Jellyfin weiterhin auf `18597`, Geräteverwaltung auf
`18595`. Compose veröffentlicht diese drei Verwaltungsports ausschließlich auf
localhost. Für die NAS-Vorschau bleibt der SSH-Tunnel nötig. Medien müssen im
Container lesbar eingebunden sein. Andere Speicherorte im Importformular zuordnen.
Bei einem bereits eingerichteten Docker-/Browser-Ziel wird zusätzlich dessen
bestehender Bibliotheksadministrator geprüft. Die Mac-Freigabe gilt ausschließlich
in der App, die den privaten Datenordner und den Serverprozess verwaltet.
Das lokale Image heißt `mutti:import-preview`; Compose baut standardmäßig `mutti:dev`.

## Was übernommen und geprüft wird

- Benutzer-IDs, lokale Passwörter und Rechte; keine Neuanlage der Konten.
- Bibliotheks- und Medien-IDs, Pfade, Metadaten, Playlists und Sammlungen.
- Favoriten, Wiedergabestatus, Resume-Position, Zähler und letztes Wiedergabedatum.
- Kompatible Server- und Benutzereinstellungen aus Jellyfins vollständiger Sicherung.

Die Importprüfung vergleicht alle gesicherten Datenbanktabellen semantisch sowie
die wiederhergestellten Medienmetadaten, Root- und Playlist-/Sammlungsdateien.
Der isolierte Restore erhält auch persistierte Felder mit privaten Settern,
unter anderem Profilbild-Zuordnungen und Anzeigeeinstellungen. Der Vergleich
normalisiert interne Versionszähler und berücksichtigt bei Benutzerrechten/-
präferenzen die tatsächlichen Benutzer/Art/Wert-Tupel.
Login-Aktivitätszeiten, Aktivitätsprotokoll sowie Geräte-/Sitzungstabellen sind
von der Inhaltsgleichheit ausgenommen. Benutzerrechte und Wiedergabedaten bleiben
vollständig im Vergleich. Alle erwarteten Tabellen müssen im Archiv vorhanden sein.

Eine zweite Quellsicherung prüft vor Aktivierung auf Änderungen an Daten,
Quelldateien und Konfiguration. Bei Abweichung stoppt der Umzug mit Wiederholhinweis.
Das ist ein kontrolliertes Wechselzeitfenster, keine kontinuierliche Replikation:
Änderungen nach der letzten Prüfung werden nicht mehr synchronisiert.

## Bewusste Anpassungen und Grenzen

- Quelle **12.1.x**, Sicherungsformat **0.2.0**. Andere Versionen werden vor dem
  Import abgelehnt; ein universeller Versionsmigrator ist nicht enthalten.
- Netzwerk, Datenbankverbindung und Log-Ausgabe bleiben unter Muttis Kontrolle.
  Alte öffentliche Listener werden nicht aktiviert. Hardwarebeschleunigung und
  Transcodingpfade werden für das Ziel zurückgesetzt und im Ergebnis ausgewiesen.
- Alte API-Schlüssel und Gerätesitzungen werden entfernt. Geräte danach neu
  anmelden/koppeln. Bestehende Mutti-Vermittler-/STUN-Einstellungen bleiben erhalten.
- Zusätzliche aktive Plugins und externe Anmeldeanbieter blockieren die Übernahme,
  solange ihre Migration nicht qualifiziert ist. Keine stille Teilübernahme.
- Medien bleiben an ihrem Ort. Fehlende Laufwerke oder fehlende Leserechte zuerst
  auflösen; Mutti zeigt den betroffenen Pfad an. Ein Dateiumzug ist nicht enthalten.
- Eine automatische Installation/Deinstallation des Fernexporters und ein
  manueller Archiv-Ausweichweg sind noch offen. Für entfernte Quellen benötigt
  diese Vorschau **HTTPS und den separat installierten Exporthelfer**.

## Quelle auf einem anderen Rechner

Der Quellserver braucht einmalig den [Mutti-Umzugshelfer](../../mutti/export/README.md).
Das DLL-Paket liegt nach dem Build in `build/export` und im Mac-App-Paket unter
`Contents/Resources/export`. Nach Installation und Jellyfin-Neustart übernimmt
Mutti Sicherung und Transfer automatisch. Nach dem Umzug Plugin wieder entfernen.

Der Download ist an den erhöhten Administratorzugang, dieselbe Gerätekennung
und ein zufälliges Einmalgeheimnis gebunden. Die Freigabe verfällt nach 30 Minuten
und wird beim Download sofort verbraucht. Keine öffentliche Archiv-URL,
keine Weiterleitung von Zugangsdaten, kein Mutti-Konto, kein Relay. Das Plugin
wird nicht in die neue Instanz übernommen; die normale Quellsicherung bleibt
unter Jellyfins Backupverwaltung. Ein entfernter HTTP-Server wird abgelehnt.

## Daten und Abbruch

Neue Daten liegen privat unter `Mutti Preview/instances/<ID>` beziehungsweise
`/config/instances/<ID>`. Erst nach erfolgreicher Prüfung und erfolgreichem Start
wird `active-instance.json` atomar umgeschaltet. Der bisherige Datenbereich bleibt
bestehen. Bei Startfehler startet Mutti den vorherigen Stand wieder.

Abbrechen oder Beenden stoppt den Import; unvollständige Instanzen werden nicht
aktiviert. Lokale Restdaten fehlgeschlagener Versuche bleiben für die Diagnose im
privaten Importordner, ohne gespeicherte Klartextpasswörter. Sicherungen enthalten
Passworthashes und werden privat gehalten. Die aktive Instanz niemals manuell
löschen; eine komfortable Wiederherstellungs-/Bereinigungsoberfläche ist noch offen.

Bei der Prüfung sind Hintergrundaufgaben und Dateiwächter ausgesetzt. Auf macOS
läuft der Prüfserver zusätzlich mit Schreibzugriff nur auf seinen eigenen
Datenbereich und ohne ausgehenden Internetzugriff. Docker erhält schreibgeschützte
Medien. Nach der Aktivierung gelten die regulären Bibliothekseinstellungen.

## Reproduzierbare Prüfungen

```sh
go test -C mutti/migrate -race ./...
go vet -C mutti/migrate ./...
swift test --package-path mutti/apps/macos
MUTTI_FRESH_IMPORT_SMOKE=1 MUTTI_TEST_EXPORT=1 MUTTI_IMPORT_REPO="$PWD" \
  go test -C mutti/migrate -race -run TestRealMigration -v -count=1
MUTTI_FRESH_IMPORT_SMOKE=1 MUTTI_TEST_CONFIGURED_TARGET=1 MUTTI_IMPORT_REPO="$PWD" \
  go test -C mutti/migrate -race -run TestRealMigration -v -count=1
python3 mutti/tests/import-package-smoke.py
```

Der vollständige Test erstellt ausschließlich eigene temporäre Server und ein
synthetisches Video. Er prüft bestehende Owner-/Viewer-Logins, Rechte, identische
Bibliotheksdaten, Playlist, Profilbild, Anzeigeeinstellungen, Favoriten,
Resume-/Wiedergabedaten, den erhaltenen
Quellserver, Export-Ticketbindung/-Einmaligkeit und Persistenz nach Neustart.
Derselbe Test ist für Linux arm64 im Container qualifiziert. Der Pakettest prüft
zusätzlich den tatsächlichen Docker-Entrypoint, veröffentlichte Loopback-Ports,
fremde Hosts/Origins und gesperrte Kopplung vor Setup-Abschluss.

Keine echten Benutzerkonten, Passwörter, Bibliotheken oder Medien für diese Tests.
Reale NAS-Mounts, andere Plattformen/Quellversionen, Internetexport und große
Bibliotheken bleiben Teil der Owner-/Release-Abnahme.

## Native Freigabe bei vorhandener Preview-Einrichtung

Der Abschlussstatus des früheren Setup-Assistenten löst auf dem Mac keine
zusätzliche Passwortabfrage mehr aus. Die App erzeugt bei jedem Start eine neue
256-Bit-Freigabe und übergibt sie ausschließlich über die private Standardeingabe
an ihren lokalen Importprozess. Ein nativer Bestätigungsdialog erlaubt den
Wechsel; der bisherige Datenbereich bleibt bestehen. Nur die Hauptseite des
Importassistenten darf diesen Dialog anfragen. Jellyfin- und Kopplungsseiten,
Unterframes, fremde Origins und Browser erhalten diese Berechtigung nicht.

Das Geheimnis wird weder im Webinhalt noch in Prozessargumenten, Umgebungsvariablen
oder Dateien gespeichert. Die normalen CSRF-/Host-/Origin-Prüfungen und die
Adminprüfung des Quellservers bleiben erforderlich. Docker akzeptiert diesen
nativen Startmodus nicht. Regressionstests decken erfundene/falsche Freigaben,
JSON-Manipulation, fehlende Wechselbestätigung und den vollständigen Import mit
unbekanntem bisherigen Zielpasswort ab.
