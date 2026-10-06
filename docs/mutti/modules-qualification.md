# Qualifizierung der Zusatzdienste

Stand: 5. Oktober 2026. **Echte Dienstprüfungen, noch keine Produktintegration.**
Mutti und kurtz bieten weiter keine fertige Foto-, Dokument- oder KI-Nutzung an.
Die Modulvorschauen bleiben entsprechend gekennzeichnet. Die Dienste werden
nicht in die bisherigen Mac-/Docker-Testpakete hineingezogen.

## Immich 2.7.5

Der bereits lokal verfügbare offizielle Server wurde per Image-Digest gesperrt
und in einem eigenen Compose-Projekt mit leerem Datenbestand betrieben.
PostgreSQL 14 mit VectorChord/pgvectors und Valkey 9 entsprechen dem gepinnten
Release-Compose. Netzwerk ausschließlich intern; keine Host-/LAN-Freigabe,
keine Verbindung zu vorhandenen Immich-Instanzen. ML und Versionsabfragen aus;
keine Gesichtserkennung oder Modelldownloads in dieser Prüfung.

Bestanden: zwei getrennte gewöhnliche Nutzer, synthetisches PNG importieren,
Original bytegleich herunterladen, Metadaten lesen, fremde Objekt-ID und
Original verweigern, fremdes Objekt aus Metadatensuche ausschließen, Duplikat
erkennen, begrenzten API-Key ohne Adminrechte verwenden und widerrufen,
Original und Sitzung nach Containerneustart erhalten.

**Technische Arbeitsentscheidung:** vorhandenen Immich-Server zunächst über
pro Profil zugeordnete, begrenzte Benutzerschlüssel anbinden. Kein globaler
Administratorschlüssel für alle Nutzer. „Metadatensuche“ ist in diesem Nachweis
keine semantische Bildsuche. Alben, HEIC/Live Photos, private Videos, Vorschaubilder,
Upload-Abbruch, Originale plus Datenbank wiederherstellen, native Galerie und
komfortabler verwalteter Mac-/NAS-Betrieb bleiben offen.

## Paperless-ngx 2.20.15

Eigene leere SQLite-Instanz und Redis 8.2 innerhalb desselben isolierten
Testprojekts. SQLite ist eine dokumentierte Upstream-Betriebsart, keine
Nachbildung des Dienstes. Zwei gewöhnliche Benutzer mit ausdrücklich
zugewiesenen Modellrechten; synthetische Text-PDF, keine Originaldokumente.

Bestanden: echter Multipart-Import → Auftrags-ID → Verarbeitung SUCCESS,
Volltextsuche, extrahierter Betrag, Eigentümer, bytegleiches Original,
fremde ID/Download/Suche verweigert, Neustartpersistenz. Der Aufgabenendpunkt
benötigt zusätzlich `view_paperlesstask`; bloßes Dokument-Leserecht genügt nicht.
Mit passenden Rechten bleibt der geprüfte Importauftrag für den fremden Nutzer
unsichtbar. Auch im Mutti-Adapter wird jede Auftrags-ID dem Profil zugeordnet.

Der Upstream-Exporter wurde ausgeführt und dessen Export **in eine zweite,
leere Instanz mit eigenen Volumes importiert**. Originalbytes, extrahierter Text,
Benutzeranmeldung und Eigentümer/Rechte erhalten. Alte API-Token werden nicht
exportiert und sind in der wiederhergestellten Instanz ungültig; neue Token
sind ausdrücklich nötig. Die laufende Quellinstanz blieb erhalten.

**Technische Arbeitsentscheidung:** vorerst den bestehenden Dienst per
Benutzertoken anbinden, Dokumente direkt dort suchen und Quellen vor Anzeige
immer erneut autorisieren. Kein eigener paralleler Volltextindex.
OCR-Bildqualität (die PDF enthält Text), mehrseitige Scans, Upload-Abbrüche,
große Bestände, native Darstellung/Import und belegte KI-Antworten bleiben offen.

## Reproduktion, Kosten und Lizenzen

`python3 mutti/tests/service-qualification.py --root build/service-qualification/NEUER-NAME`
verweigert existierende Testordner und verwendet ausschließlich bereits
vorhandene, exakt bezeichnete Images. Es startet nur das eigene Projekt und
stoppt es am Ende; synthetische Volumes und Nachweise bleiben erhalten.

[Maschinenlesbare Ergebnisse und Digests](evidence/services-2026-10-05.json).
Vollständige lokale Nachweise:
`/Users/ai/workspace/mutti/build/service-qualification/2026-10-05-v7/`.
`compose.json` enthält nur synthetische, aber private Testgeheimnisse (0600)
und gehört nicht in öffentliche Nachweise. Vorläufe v1–v4 dokumentieren
korrigierte Testkonfiguration, Erreichbarkeit und fehlendes Aufgabenrecht;
sie zählen nicht als bestandene Gesamtabnahme. v5 bestand die API-Strecke,
v6 zusätzlich den separaten Paperless-Restore. Bei der Abschlusskontrolle blieb
der separat aktivierte Restore-Container nach dem normalen Compose-`down` noch
übrig. Er wurde ausschließlich anhand seines eigenen Projektlabels beendet.
Der Runner nimmt jetzt das Restore-Profil ausdrücklich in die Bereinigung auf
und prüft anschließend, dass kein Container dieses Projekts mehr existiert.
**v7 besteht die komplette Strecke einschließlich dieser Bereinigungsprüfung**;
alle synthetischen Volumes bleiben erhalten.

Immich: AGPL-3.0; Paperless-ngx: GPL-3.0. Alle verwendeten Image-Digests sind
belegt. Laufzeiten, Datenbanken, Bild-/PDF-/OCR-Werkzeuge und Modelle benötigen
vor Produktpaketierung weiterhin ein vollständiges transitives Inventar mit
passenden Quellen. Vorhandene Images wurden lokal wiederverwendet; keine
Buchungen oder bezahlten APIs. Ein verwalteter Dienst bringt zusätzliche
Datenbanken, Wartung und Speicherbedarf mit, auch wenn keine Lizenzgebühr anfällt.

Primärquellen:
[Immich Release-Compose](https://github.com/immich-app/immich/blob/v2.7.5/docker/docker-compose.yml),
[Immich API-Vertrag](https://github.com/immich-app/immich/blob/v2.7.5/open-api/immich-openapi-specs.json),
[Paperless API](https://github.com/paperless-ngx/paperless-ngx/blob/v2.20.15/docs/api.md),
[Paperless Administration](https://github.com/paperless-ngx/paperless-ngx/blob/v2.20.15/docs/administration.md).
