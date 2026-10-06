# Qualifikation v6 – Messungen und Befunde (06.10.2026)

Protokoll und Schwellen: [qualification-v6-protocol.md](qualification-v6-protocol.md).
Branch `codex/mutti-p2-qualification` (Worktree `/Users/ai/workspace/mutti-p1`).
Alle Daten synthetisch (isolierte Immich-/Paperless-Testumgebung, synthetische
Jellyfin-Bibliothek). **Es ist noch nichts signiert oder freigegeben.**

## Gemessene Bereitstellung (Attestation)

| Feld | Wert |
| --- | --- |
| Hardware | `Mac16,9`, Apple M4 Max, 12P+4E, GPU 40, 128 GiB |
| OS | macOS 27.0 (26A428) |
| Engine-Verzeichnis | gebündeltes Ollama 0.32.13 inkl. MLX-Kernel, Digest `9dc018e018b0…`, Netzsperre verifiziert |
| Modell | `qwen3.8:27b-mlx` (Katalog-Digest), Kontext 8192, Temperatur 0, Thinking aus |
| Hub | reproduzierbar gebaut (`-buildvcs=false`); Digest wechselt mit jeder Codeänderung |

Hub-Reproduzierbarkeit geprüft: Neubau nach einem reinen Doku-Commit ergab
denselben Digest; die Ad-hoc-Signatur der App verändert die Hub-Datei nicht.

## Entwicklungsregression (Casting-Satz v6-dev, 60 Fälle, nicht Gate)

| Lauf | Stand | Ergebnis |
| --- | --- | --- |
| v6-dev-1 | englischer Vertrag v6, vor Korrekturen | 60/60 |
| v6-dev-2/3 | + `retry`-Korrektur für alle Suchen | 59/60 (U02 listet nach breiterer Dokumentsuche fremde Treffer) |
| v6-dev-4 | `retry` nur für Foto-/Filmsuche | 60/60 |
| v6-dev-5 | + Laufzeitfilter „länger als“, Korrektur erfundener Marken | 60/60 |

Zum Vergleich v5 (deutscher Vertrag): 177/180 über drei Wiederholungen.
Median 45–58 Token/s, p95 Werkzeugantwort 13–15 s (zeitweise paralleler Build).

## Produktqualifikation auf echten Adaptern

Paket gebaut aus dem jeweiligen Stand; `mutti-hub qualify` aus der App, drei
Wiederholungen je Fall, deterministisch (identische Ergebnisse je Wiederholung).

### real-de-v1 (danach gesehen) – 69/84

| Aufgabe | Ergebnis | Grund |
| --- | --- | --- |
| content.assist | 69/84, 3 kritisch | F03, Nichtwissen C02 |
| documents.read | 24/27 | bestanden |
| media.favorite | 6/9, 3 kritisch | F03 kein Vorschlag |
| media.read / media.search / photos.search | 3/6, 24/30, 9/12 | C05 (Bewertung zu eng), F03, P02 |

Befunde → Korrekturen (Commit `42f3897b61`): Favoritenwunsch „zu meinem
Favoriten“ nicht erkannt; Behauptung „in deinen Daten nicht gefunden“ ohne
Suche; Dokumentsuche mit zwei Begriffen ohne Treffer (Schreibvariante) – jetzt
Rückfall auf einen der Begriffe; Modell bot neue Suche an statt sie
auszuführen – jetzt `retry` für Foto-/Filmsuche; Bewerter für Nichtwissen und
Zeitformate zu eng.

### real-de-v2 (danach gesehen) – 78/84

| Aufgabe | Ergebnis |
| --- | --- |
| documents.read | 24/24 – Schwelle erfüllt |
| media.favorite | 9/9 – erfüllt |
| media.read | 6/6 – erfüllt |
| media.search | 27/30 – erfüllt |
| photos.search | 15/15 – erfüllt |
| content.assist | 78/84 – **nicht** erfüllt: N22 schreibt in einer Fähigkeitserklärung eine Beispiel-Quellenmarke (vom Produkt entfernt, laut Protokoll aber kritisch) |

Latenz: erste Ausgabe p50 0,6 s / p95 1,1 s, vollständige Antwort p50 2,5 s /
p95 5,8 s; Median 51 Token/s; Engine-Speicher bis 15,0 GiB.
Befunde → Korrekturen (Commit `cdfb464f3a`): Filter „länger als“ fehlte (N02);
neue Korrektur `markers` gegen nicht ausgegebene Quellenmarken.

### Offen (Gate)

- **real-de-v3** (frisch, eingefroren) und **real-en-v1** (ungesehen; ein Lauf
  auf dem Vorstand wurde abgebrochen und nicht gelesen,
  `build/qualification-en-1-aborted-unread/`) mit dem aktuellen Paket.
- Danach Review und gegebenenfalls Signatur passender Kandidaten; erst dann
  Produkt-E2E mit eingeschalteter KI (`module-package-smoke.py --model
  qwen3.8:27b-mlx --model-source adopt --adopt-from … --expect-qualified`).
- Wiedergabe unter KI-Last: erster Versuch der Messsonde scheiterte an ffmpeg
  (Sonde gibt jetzt den Fehler aus); keine Aussage zur Wiedergabe getroffen.
- v6-Holdout (44 Fälle, ungesehen) noch nicht gelaufen.

## Grenzen

Eine Freigabe gilt exakt für dieses Mac-Modell und diesen OS-Build; jedes
macOS-Update und jeder neue Hub-Build verlangt eine neue Messung. Synthetische
Daten, ein Rechner, keine echten Nutzerdaten. Der Lüfter des Entwicklungs-Macs
begrenzt nächtliche Messläufe; Läufe nur auf Ansage.
