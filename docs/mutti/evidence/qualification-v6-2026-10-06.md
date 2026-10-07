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

### real-de-v4 (07.10.2026, danach gesehen) – Paket `c365b6256`

Paket sauber gebaut (Komponentenliste geprüft, Hinweise enthalten);
`mutti-hub qualify` aus der App, 3 Wiederholungen, deterministisch.

| Aufgabe | Ergebnis |
| --- | --- |
| content.assist | 75/84 (89 %) – Schwelle erfüllt |
| media.search | 33/33 – erfüllt |
| media.read | 9/9 – erfüllt |
| media.favorite | 9/9 – erfüllt |
| documents.read | 21/27 (78 %) – **nicht** erfüllt |
| photos.search | 12/15 (80 %) – **nicht** erfüllt |

0 kritisch, 0 ungültige Marken, 0 Engine-Fehler, Nichtwissen 6/6. Latenz:
erste Ausgabe p50 0,6 s / p95 1,0 s, vollständig p50 2,5 s / p95 7,0 s;
Median 56 Token/s; Engine-Speicher bis 16,1 GB. Wiedergabe leer und unter
KI-Last: alle fünf Clips abspielbar (progressiv).

Fehlschläge (je alle drei Wiederholungen): K16 – Antwort richtig, zitiert
aber die OCR-Kopie des Scans, die der Satz nicht zuließ (**Fehler im
Satz**); K20 – Volltextsuche findet „Rechnung“ nicht in „Arztrechnung“
(**Produktlücke**, behoben: Teilwortsuche); K25 – Datumssuche „Herbst“
liefert zwölf unbeschriftete Fotos vom 06.10. aus früheren nativen
Testläufen, „Herbstlaub“ liegt außerhalb der ersten zwölf (**Testumgebung**,
Modell sagt korrekt, dass es mehr gibt). Nebenbefund: In rund 15 % aller
aufgezeichneten Antworten (auch v6-dev und de-v1/v2) stand ein
`</think>` mit wiederholter Antwort – bisher von keinem Bewerter erkannt;
behoben (`f1ae8dbd43`).

### real-en-v2 (07.10.2026, danach gesehen) – Paket `3760794b2d`

| Aufgabe | Ergebnis |
| --- | --- |
| media.search / media.read / media.favorite | 33/33, 9/9, 9/9 – erfüllt |
| documents.read | 21/27, Nichtwissen 0/3 – **nicht** erfüllt |
| photos.search | 6/15 – **nicht** erfüllt |
| content.assist | 69/84 – **nicht** erfüllt |

0 kritisch, 0 ungültige Marken, 0 Engine-Fehler; Median 62 Token/s, p95
vollständige Antwort 10,2 s, Engine bis 16,1 GB; Wiedergabe leer/unter
KI-Last in Ordnung. Ursachen: Fotosuche ohne Smart Search verglich die ganze
Anfrage mit der Beschreibung („lake sunset“ ≠ „Lake at sunset“,
„skateboarding“ ≠ „skateboard“) und das englische Wiederholungsangebot wurde
nicht erkannt (**Produktlücke**, behoben: Wort-/Stammsuche, live geprüft);
„found no matching documents“ wurde nicht als „nichts gefunden“ erkannt
(**Bewerterlücke**); zwei Fälle mehrdeutig für die Testdaten (zwei
Versicherungsscheine; Herbst-Datumssuche). Testdatenbefund: das englische
Skateboard-Video ist bytegleich mit dem deutschen Fahrradvideo; Immich hat
beide zu einem Objekt zusammengeführt.

v6-Holdout auf `c365b6256`: 129/132 (ein Fall mit richtiger Antwort, die an
Markdown scheiterte; Bewerter korrigiert).

### real-de-v5 (07.10.2026, Gate) – Paket `426e3fad81` – **alle Aufgaben erfüllt**

| Aufgabe | Ergebnis |
| --- | --- |
| content.assist | 84/84 |
| media.search | 33/33 |
| media.read | 9/9 |
| media.favorite | 9/9 |
| documents.read | 27/27 |
| photos.search | 15/15 |

0 kritisch, 0 ungültige Marken, 0 Engine-Fehler, Nichtwissen 6/6, keine
Reasoning-Tags. Latenz: erste Ausgabe p50 0,6 s / p95 1,2 s, vollständig
p50 2,5 s / p95 6,3 s; Median 60 Token/s; Engine bis 16,1 GB. Wiedergabe
leer und unter KI-Last: alle Clips abspielbar. Entwicklungsregression vor
dem Lauf: v6-dev-9 60/60 auf demselben Harness.

### real-en-v3 (07.10.2026, danach gesehen) – Paket `426e3fad81`

| Aufgabe | Ergebnis |
| --- | --- |
| media.search / media.read / media.favorite | 33/33, 9/9, 9/9 – erfüllt |
| photos.search | 15/15 – erfüllt (Wort-/Stammsuche wirkt) |
| documents.read | 21/27, Nichtwissen 0/3 – **nicht** erfüllt |
| content.assist | 78/84, Nichtwissen 3/6 – **nicht** erfüllt |

0 kritisch, 0 ungültige Marken; Median 60 Token/s, p95 vollständig 5,0 s.
N18: die Teilwortsuche fand über „Vertrag“ den Mietvertrag, das Modell
nannte ihn mit Quelle, obwohl es ihn als unpassend erkannte; N20: das Modell
suchte für eine englische Frage auf Deutsch und fand die englische
Zahnarztrechnung nicht (wie schon en-v2 M16). Behoben in `a4f1978460`
(alle Begriffe müssen enthalten sein; Regel 3 mehrsprachig).

**Release-Kandidat `426e3fad81`** (unverändert gesichert unter
`build/release-candidates/426e3fad81/`): Deutsch alle sechs Aufgaben
bestanden, Englisch Medien und Fotos bestanden; nichts signiert.

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
