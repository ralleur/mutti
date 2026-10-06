# Mutti Serververwaltung (UX3-Mutti / MK-008)

Auftrag vom 05.10.2026: Nach Einrichtung und Import öffnet Mutti eine eigene
Serververwaltung. kurtz bleibt der Inhaltsclient. Grundlage ist Ralfs dunkles
Dashboard-Konzept mit kompakter Mutti-Marke, Sora und gelben Hauptaktionen.

## Umfang dieses Inkrements

- Standardziel `/web/#/mutti` nach Anmeldung, Einrichtung und Import; auch die
  Mac-Hülle öffnet diesen Weg. Vor Einrichtung bleibt die bestehende Erstwahl.
- Sechs Bereiche: Übersicht, Bibliotheken, Geräte, Module, Speicher, Einstellungen.
- Echte API-Werte für Bibliotheken/Einträge, Datenträger, Server und Scanstatus.
  Ausfälle oder fehlende Messungen werden als unbekannt angezeigt. Mehrere
  Ordner auf demselben Datenträger werden nicht zu einer erfundenen Kapazität addiert.
  Bei `StorageType=Unknown` wird keine Kapazität behauptet: Docker Desktops
  virtiofs meldete im Test über .NET DriveInfo um Faktor 256 falsche Bytewerte
  (Transferblock 1 MiB, Dateisystemblock 4 KiB). Die Oberfläche markiert diese
  Messung als nicht verlässlich; eine Backend-Korrektur bleibt separat offen.
- Bibliothek anlegen, Ordnerwahl auf dem Mac, Bibliotheken neu prüfen,
  Servername ändern unter Erhalt der übrigen Konfiguration.
- QR-Einladung, bestehende Geräte, Profilanlage, Freigabe und Widerruf über
  den vorhandenen Administratorzugang. Gerätezahlen zählen Freigaben, keine
  vorgetäuschten Online-Sitzungen. Direkter Fernzugriff behält die bestehende
  native Konfiguration und die Foundation-Grenzen.
- Erweiterte Medien-/Benutzerverwaltung bleibt gezielt erreichbar.
- Fotos (Immich-Kandidat), Dokumente (Paperless-ngx), lokale KI
  (Ollama-Kandidat) und Zuhause (Dienst offen) als eigene, bedienbare Vorschauen.
  Sie sind ausdrücklich **noch nicht angebunden**. Entwürfe gelten nur in der
  aktuellen Ansicht, werden nicht gespeichert und starten keine Dienste.
  Modulfreigaben und gemeinsamer Updatebetrieb bleiben gekennzeichnete
  Ausbauaufgaben. Dies liefert kein neues Modulbackend aus.

## Architektur und Grenzen

Der Web-Bereich liegt isoliert in `src/mutti/server` im Mutti-Web-Repo. Stabile
Modul-IDs bilden die Navigation; spätere Adapter ersetzen gezielt Vorschauen.
Authentifizierung und Administratorrechte kommen aus dem vorhandenen Jellyfin-
Server. Nichtadministratoren erhalten keinen Verwaltungsdatenabruf.

`GET /Mutti/Management` nennt paketkonfigurierte Einstiegspunkte.
`POST /Mutti/Connect/{action}` akzeptiert nur die sechs vorhandenen Besitzer-
Aktionen `state`, `invite`, `qr`, `approve`, `revoke`, `profile`. Der Server
leitet den vorhandenen Anmeldetoken ausschließlich an seinen festen Loopback-
Connect-Port weiter. Es gibt keine frei wählbare Zieladresse, keine Redirects,
keinen zusätzlichen Owner-Token im Browser und keine neue Kontenverwaltung.
Die Bridge verlangt die bestehende Elevation-Policy, begrenzt Requests auf
16 KiB und Antworten auf 2 MiB; Connect prüft Besitzerrechte nochmals.

Die Mac-Brücke erlaubt nur der lokalen Verwaltungsseite im Hauptframe,
Import oder Fernzugriffseinstellungen zu öffnen. Bestehende Host-/Origin-
Grenzen, Importfreigabe, Datentrennung und Day-1-Regeln bleiben in Kraft.

## Prüfung

- Web: TypeScript, gezieltes ESLint/Stylelint und Modelltests für unbekannten
  Speicher, gemessene Nullwerte sowie gültige/ungültige Detailrouten.
- API-Build, Go-Migrationstests, Swift-Tests und vollständige Mac-/Docker-Builds.
- `python3 mutti/tests/management-package-smoke.py`: frische synthetische
  Docker-Instanz; Setup, Owner-/Viewer-Rechte, echte QR-Bridge, Fehlerstatus,
  Größenlimit, fremde Hosts/Origins, Bibliotheksanlage, Speicher, Servername,
  Scanstatus. Kein Zugriff auf vorhandene Quellbibliotheken.
- `--keep` hält ausschließlich diesen Testcontainer für anschließende Browser-
  Prüfung offen. Die synthetischen Zugangsdaten liegen mit Modus 0600 im
  temporären Verzeichnis. Danach den ausgegebenen Testcontainer entfernen.

Der konkrete Abnahmestand wird in `status.md` ergänzt. Reale NAS-Hardware,
WAN-Netze und vollständige VoiceOver-Abnahme bleiben eigene Gates.


## Erweiterung: Rechte und lokale Wiederherstellung

„Geräte“ öffnet für jedes nicht administrative Profil einen echten Rechtedialog:
Wiedergabe, alle Bibliotheken einschließlich künftiger oder ausdrücklich gewählte
Bibliotheken. Keine Auswahl bedeutet keinen Bibliothekszugriff. Vor dem Speichern
wird die aktuelle Policy erneut geladen; Altersregeln, Zeitpläne und andere
bestehende Felder bleiben erhalten. Die Serverantwort wird danach geprüft.

„Speicher“ enthält echte lokale Sicherungen mit Größe, Erstellungszeit und
Zeitpunkt einer bestandenen Wiederherstellungsprobe. Erstellen, Prüfen und
Wiederherstellen sind serverseitige Aufträge; die Ansicht kann geschlossen werden.
Fehler und nach Neustart unterbrochene Aufträge erhalten eigene Zustände.

`POST /Mutti/Maintenance/{action}` lässt ausschließlich `state`, `backup`,
`verify`, `restore` zu. Dieselbe lokale, begrenzte Bridge verlangt Besitzerrechte;
der Manager kontrolliert sie zusätzlich und vor tatsächlicher Umschaltung erneut.
Kennwörter werden nur für die Anmeldung in der isolierten Sicherung verwendet,
nicht gespeichert. [Umfang, Aufbewahrung und Grenzen](maintenance.md).
