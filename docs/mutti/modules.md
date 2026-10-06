# Mutti-Module: lokale KI, Fotos und Dokumente

Stand: 6. Oktober 2026. **Adapter und Clientquellen implementiert; keine
vollständige native/Paket-/Release-Abnahme.** Maßgeblich:
[Bestand und Evidenz](baseline-2026-10-06.md), [Status](status.md),
[P0-Verträge](contracts.md) und [P0-Prüfungen](evidence/p0-2026-10-06.md).
Die zuvor verlinkten Modul-E2E-/Casting-v2-Dokumente vom 6. Oktober lagen nicht
vor; daraus wird kein bestandener Nachweis abgeleitet.

## Architektur

```text
kurtz ── bestehender verschlüsselter Connect-Tunnel ── Mutti Connect ──┬─ Jellyfin (Medien)
                                                     (Gerät → Profil)  └─ mutti-hub /mutti/hub/v1/…
Mutti-Web (Besitzer) ── Jellyfin-Anmeldung ── /Mutti/Hub/admin/… (Allowlist) ── mutti-hub
mutti-hub ── Profilkonto ──> Immich        (Fotos, private Videos)
          ── Profilkonto ──> Paperless-ngx (Dokumente, OCR)
          ── Profilsitzung ─> Jellyfin     (Filmsuche, Favoriten)
          ── Loopback ──────> lokale Engine (Ollama, auf macOS per OS-Sandbox ohne Netz)
```

- `mutti/hub` ist ein eigener Go-Dienst (GPL-2.0-or-later), nur auf Loopback.
  Der Manager startet ihn nach abgeschlossener Einrichtung und startet ihn bei
  Absturz mit Pause neu. Jellyfin, Connect und Wiedergabe hängen nicht von ihm ab.
- **Keine zweite Kopplung, kein offener Dienstport.** Geräte erreichen den Hub
  ausschließlich über `/mutti/hub/v1/` im bestehenden Tunnel. Connect setzt dort
  wie bei Jellyfin die gerätegebundene Profilsitzung ein; vom Client gelieferte
  Identität, Tokens oder Peer-Header werden verworfen. Ein pro Managerlauf
  erzeugtes Peer-Geheimnis belegt dem Hub, dass die Geräteangabe von Connect stammt.
- Besitzeraktionen laufen über `POST /Mutti/Hub/{admin/…}` im Jellyfin-Server:
  Administratorrichtlinie, feste Allowlist, 16 KiB Anfragen, 2 MiB Antworten,
  festes Loopback-Ziel. Gerätezugriffe auf `admin/` werden in Connect gesperrt.
- Jede Hub-Anfrage wird mit der Jellyfin-Sitzung des Aufrufers geprüft
  (`/Users/Me`, höchstens 3 s zwischengespeichert). Gerätewiderruf meldet
  zusätzlich die Jellyfin-Sitzung des Geräts ab.

## Rechte, Widerruf und Datenschutz

- Je Modul: eingerichtet, eingeschaltet, Freigabe pro Profil. Fotos/Dokumente
  zusätzlich ein **eigenes Dienstkonto pro Profil**. Der Besitzer meldet das
  Konto einmal an; Mutti erzeugt daraus einen profilgebundenen Schlüssel
  (Immich: nur Lesen/Ansehen/Download/Upload/Alben; Paperless: Benutzertoken)
  und speichert das Passwort nicht. Administrator-/Superuser-Konten und doppelt
  zugeordnete Konten werden abgewiesen. Immich und Paperless erzwingen Eigentum.
- Entzogene Freigabe, Kontolösung oder Modulabschaltung gilt sofort: laufende
  Ereignisströme, Bild-/Video-/Dokumentdownloads und KI-Generierungen werden
  beendet. Laufende Ströme prüfen Sitzung und Freigabe zusätzlich alle 4 s.
- Dienstadressen müssen auf Loopback oder ein privates Netz zeigen, auch nach
  DNS-Auflösung; keine Redirects, kein Proxy. **Kein Cloud-/Relay-Ausweichweg.**
- Antworten tragen `Cache-Control: no-store`. kurtz nutzt eine ephemere
  URLSession ohne Datenträgercache; Fähigkeiten werden beim Profilwechsel verworfen.

## Lokale KI

- **Harness:** eigener schlanker Werkzeugkreis in Go (`harness.go`) mit
  versioniertem, derzeit deutschem Promptvertrag `mutti-assistant-v4`.
  Begrenzte Korrekturen, je höchstens einmal pro Run und für den Nutzer nur als
  zurückgezogener Entwurf (`reset`) sichtbar: Werkzeug zuerst (auch bei
  Ausweichen wie „soll ich suchen?“), eigene Archivsuche, wenn das Modell zu
  eigenen Daten weiter nicht sucht, fehlender Favoritenvorschlag, behauptete
  Änderung (danach fester ehrlicher Bestätigungssatz), nicht-deutsche Antwort,
  fehlende Quellenmarke trotz Treffern. Auslöser sind nur Nutzerfrage und
  Modellentwurf, nie Werkzeugdaten oder Anhangstext.
  Pydantic AI ist bisher nur paketgeprüft, ein Funktionsvergleich bleibt offen.
  Englischer Produktionsvertrag erfordert neue Version und neue Messung.
- **Werkzeuge serverseitig:** `search_movies` (Stichwort, Genre, Jahr,
  gesehen/ungesehen, Laufzeit „unter“ oder „höchstens“, Sortierung inkl. längste
  zuerst), `get_movie`, `propose_favorite`
  (Jellyfin mit Profilsitzung), `search_documents`, `read_document` (Paperless),
  `search_photos` (Immich). Nur Werkzeuge freigegebener Module werden angeboten.
- **Quellen:** Der Server vergibt pro Gespräch Marken `Q1…`; das Modell sieht
  keine Objekt-IDs. Nicht vergebene Marken werden entfernt, `(Q1)` und
  `(Quelle Q1)` normalisiert.
  Quellen werden vor dem Öffnen erneut berechtigt (`GET ai/sources/{gespräch}/{marke}`).
- **Änderungen:** Das Modell kann nur vorschlagen. Bestätigung/Ablehnung ist eine
  eigene authentifizierte Aktion; Wiederholungen entschiedener Vorschläge liefern das gespeicherte Ergebnis;
  dieses wird nach der Aktion zurückgelesen (`confirmed`, `failed`, `outcome_unknown`). Ein dauerhaftes Ausführungsjournal
  für Abstürze zwischen Seiteneffekt und Speicherung bleibt P1; keine allgemeine
  Exactly-once-Garantie. P0 prüft Aufgabenqualifikation und Vorschlagsherkunft
  erneut vor der Wirkung; alte Vorschläge ohne Nachweis sind gesperrt.
- **Gespräche:** pro Profil als private Dateien, Runs mit Zuständen
  `queued/running/completed/cancelled/failed/interrupted`, Idempotenzschlüssel
  für Nachricht und Retry, SSE mit Cursor-Wiederaufnahme, nach Neustart
  `interrupted` statt stiller Fortsetzung. Eine Generierung gleichzeitig (Warteschlange).
- **Anhänge:** Text/Markdown/CSV/PDF bis 20 MB, lokale Textextraktion der
  PDF-Textschicht; Scans werden als „kein Text“ gemeldet. Kein automatischer Import.
- **Engine:** Mac-Paket enthält Ollama 0.32.13 (MIT, SHA-256 gepinnt, arm64,
  nur llama.cpp-Runner). Mutti betreibt eine eigene Instanz mit eigenem
  Modellordner, `OLLAMA_NO_CLOUD=1`, eigener `HOME`, und unter macOS-Sandbox
  ohne ausgehende Verbindungen außer Loopback; die Sperre wird per Socketprobe
  verifiziert. Nur ein vom Besitzer ausgelöster Download startet kurzzeitig einen
  ungesperrten Downloader; danach wird der Manifest-Digest geprüft.
  Docker enthält keine Engine; dort wird eine eigene Engine im Heimnetz angegeben.
- **Modelle:** nur Katalogeinträge mit Manifest-Digest (`catalog.go`), Eignung
  nach Arbeitsspeicher, Qualifikationsstatus aus dem Casting.
  Vorhandene Mac-Ollama-Modelle können digestgeprüft übernommen werden.
  **Installation ist keine Qualifikation:** Nachrichten/Retry/Run-Start,
  Werkzeugangebot/Dispatch und Bestätigung prüfen den passenden Nachweis.
  Es existiert kein produktiver Pass; die KI-Ausführung bleibt derzeit mit
  `qualification_required` gesperrt. Synthetisches Casting bleibt getrennt möglich.
  Runtime-Attestation und vertrauenswürdige Evidenzübernahme vor erster Promotion
  sind P2; unbekannte Engine/Hardware wird nicht aus einer RAM-Empfehlung abgeleitet.

## Fotos (Immich 2.7.5) und Dokumente (Paperless-ngx 2.20.15)

- Fotos: Zeitleiste, Videos, Alben (inkl. geteilter), Suche (Immich-Smart-Search,
  wenn deren ML aktiv ist, sonst Beschreibung/Dateiname/Ort und Zeitraum),
  Einzelansicht, Vorschau, Original, Video mit Range, Upload mit Duplikaterkennung.
  Keine Löschung, keine automatische Fotosicherung (iOS-Hintergrund) in diesem Stand.
- Dokumente: Liste, Volltextsuche mit bereinigtem Textauszug, Metadaten,
  Vorschau-PDF, Original, erkannter Text, Import mit Auftragsverfolgung bis
  OCR-Abschluss/Fehler/Duplikat. Aufträge gehören dem importierenden Profil;
  die Paperless-Aufgaben-API wird nie direkt weitergegeben.
- Betriebsart dieses Stands: **vorhandenen Dienst anbinden.** Mutti installiert
  oder aktualisiert Immich/Paperless noch nicht selbst.

## Daten, Sicherung und Wiederherstellung

`<Mutti-Datenordner>/hub/`: `hub.json` (Konfiguration und profilgebundene
Dienstschlüssel, 0600), `ai/conversations/`, `ai/attachments/`,
`document-tasks.json`, `ai/models/` (Modelle), Logs. Der Ordner gilt für alle
Jellyfin-Instanzen, damit Import/Restore der Bibliothek Module nicht verwirft.
Sicherungen enthalten die Moduldaten **ohne** Modelldateien, jede Datei mit
SHA-256; die Wiederherstellung legt den vorherigen Stand als
`hub-before-restore-<Zeit>/` ab. Immich-/Paperless-Bestände selbst sichert der
jeweilige Dienst; ihre Wiederherstellbarkeit ist in der
[Dienstqualifikation](modules-qualification.md) belegt, nicht durch Mutti automatisiert.

## API v1 (Auszug)

Gerät (`/mutti/hub/v1/`): `capabilities`; `ai/conversations` (GET/POST),
`ai/conversations/{id}` (GET/PATCH/DELETE), `…/messages`, `…/attachments`,
`ai/runs/{id}/events|cancel|retry`, `ai/proposals/{id}/confirm|reject`,
`ai/sources/{gespräch}/{marke}`; `photos/assets|search|albums|assets/{id}/{thumbnail|preview|original|video}`;
`documents`, `documents/{id}`, `documents/{id}/{thumbnail|preview|original}`, `documents/tasks`.
Fehler: `{"code","message"}` mit stabilen Codes (`not_configured`, `disabled`,
`forbidden`, `not_linked`, `unavailable`, `busy`, `engine_not_ready`, …).
Besitzer (`/Mutti/Hub/admin/…`): `state`, `enable/{modul}`, `grants`,
`service|link|unlink/{photos|documents}`, `ai/engine`, `ai/models/{pull|adopt|select|remove}`.

## Bekannte Grenzen dieses Stands

- Keine verwaltete Installation/Aktualisierung von Immich/Paperless durch Mutti;
  kein iOS-Hintergrund-Fotobackup; keine Immich-Löschung/Bearbeitung.
- Docker: keine mitgelieferte Engine und keine OS-Netzsperre für eine externe Engine.
- Medienvorrang ist durch Warteschlange und eine Generierung gleichzeitig
  angenähert; keine Kopplung an laufende Transcodes. Lastprüfung (AT-22) offen.
- Reale iPhone/iPad, NAS, WAN und Langzeitbetrieb wie bei der Foundation offen.
