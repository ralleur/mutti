# Harness v5 mit Qwen3.8 27B (MLX) — 06.10.2026

Protokoll und Freeze: [casting-v5-rubric.md](casting-v5-rubric.md). Vorstufe:
[v4-Messung](model-comparison-v4-2026-10-06.md). Synthetische Daten,
**Entwicklungsmessung, keine Produktfreigabe**.

Umgebung wie v4: Apple M4 Max 128 GB, Homebrew Ollama 0.32.13 (MLX-Runner),
`qwen3.8:27b-mlx` aus `build/model-casting/store-mlx`, Kontext 8192, Temperatur 0,
Seed 42, Thinking aus, 3 Wiederholungen, `networkLock: unverified`. Quellstand:
Basis-HEAD `f98bab2be48f326741cec1a8dbef9959ec306bdf` plus uncommittete
Änderungen auf `codex/mutti-foundation`.

## Änderungen gegenüber v4 (Prompt `mutti-assistant-v5`)

1. Automatische Archivsuche nur bei Fragen, nicht bei Aufforderungen (v4: G06).
2. Keine Zitier-Korrektur, wenn der Entwurf meldet, dass nichts Passendes
   gefunden wurde (v4: HU02).
3. Promptregel 4: ohne passenden Treffer keine Quellenmarke und keine
   Aufzählung nicht gefragter Treffer (v4: U01/U02).

Werkzeugvertrag und Bewerter unverändert. `go test ./...` grün (neu:
`TestHarnessV5Boundaries`).

## Ergebnisse

| Satz | v4 (27B MLX) | **v5 (27B MLX)** | Werkzeuge | Dokumente | Nichtwissen | Fremdanw. | Dialog auto | p95 erste Ausgabe | p95 Werkzeugantwort | Median tok/s |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Entwicklung (60×3) | 172/180 | **177/180** | 60/60 | 45/45 | 12/15 | 30/30 | 30/30 | 2,04 s | 14,03 s | 37,0 |
| Regression = v4-Holdout (44×3, gesehen) | 126/132 | **129/132** | 42/45 | 33/33 | 12/12 | 27/27 | 15/15 | 1,40 s | 8,18 s | 40,1 |
| **Neuer v5-Holdout (44×3)** | — | **129/132** | 42/45 | 33/33 | 12/12 | 27/27 | 15/15 | 2,23 s* | 19,39 s* | 26,6* |

Keine Engine-Fehler, keine ungültigen Quellenmarken in allen drei Sätzen.
Harness-Korrekturen: Entwicklung 3× `tool_first` (G06, ohne Folgesuche),
Holdout 3× `citation`, Regression keine.

\* **Latenz des ersten Holdout-Laufs nicht belastbar:** Während der Messung lief
eine virtuelle Maschine (Virtualization.framework) mit rund 465 % CPU,
Systemlast um 10; die Generierungsrate schwankte zwischen den Wiederholungen
(Median 23,2 / 22,7 / 41,6 tok/s).

### Wiederholung nach Pausieren der VM (06.10.2026, 19:49–20:06)

Unveränderter Freeze (harness.go `00d367f4…`, `build/mutti-cast-v5` `a2b7e7ca…`).
Systemlast alle 30 s protokolliert (`build/model-casting/2026-10-06-v5-rerun-load.log`):
1-Minuten-Last Median 4,0; eine Spitze bis 40 von 19:50 bis 19:53 (VM-Prozess
beim Pausieren, ~166 % CPU), danach ~4. Parallel lief anfangs ein `xcodebuild`.

| Satz | Ergebnis | Fehler | p95 erste Ausgabe | p95 Werkzeugantwort | Median tok/s |
| --- | ---: | --- | ---: | ---: | ---: |
| v5-Holdout (Wiederholung) | 129/132 | VT15 ×3 | 1,07 s | **6,02 s** | 53,1 |
| Entwicklung (Wiederholung) | 177/180 | U02 ×3 | 1,19 s | **8,14 s** | 52,0 |

Die Qualitätsergebnisse sind in beiden Wiederholungen identisch zum ersten
Lauf. Die Latenzgrenzen (≤ 3 s, ≤ 15 s, ≥ 15 tok/s) sind klar erfüllt.
Gemessen wurde auf einem normal genutzten, nicht vollständig ruhigen Rechner
und mit Homebrew-Ollama, nicht mit der gebündelten Engine.

Rohdaten: `build/model-casting/2026-10-06-v5-dev-27b-mlx/`,
`2026-10-06-v5-regression-27b-mlx/`, `2026-10-06-v5-holdout-27b-mlx/`,
Wiederholungen `…-holdout-27b-mlx-rerun/`, `…-dev-27b-mlx-rerun/`.

## Abschluss der drei offenen Punkte (06.10.2026, 20:15–20:47)

### 1. Verifizierte Netzsperre

Befund: Die OS-Sperre (`sandbox-exec`, Profil `deny network-outbound` außer
Loopback/Unix-Socket) wirkte schon vorher — manuell: `nc` zu 1.1.1.1:443 in der
Sandbox scheitert, ohne Sandbox gelingt es. Die Prüfung `verifySandbox()` rief
`nc -z` aber ohne `-v` auf; ohne Ausgabe fehlte das erwartete „not permitted“,
deshalb stand jeder Lauf auf `unverified`. Korrektur: `nc -v` (Sandbox:
„Operation not permitted“, ohne Sandbox: Timeout). Neuer Test
`TestSandboxDeniesOutboundOnMac`. Alle drei Läufe unten: `networkLock: verified`.
Der MLX-Runner (`ollama runner --mlx-engine`) ist Kindprozess des gesperrten
`ollama serve` und erbt die Sperre.

### 2. Gebündelte Engine

`mutti/packaging/fetch-ollama.py` ließ die MLX-Kernel bisher bewusst weg. Für
arm64 werden `mlx_metal_v3` und `mlx_metal_v4` jetzt aus dem unveränderten,
per SHA-256 gepinnten Archiv (`71efd44f…`, Ollama 0.32.13) übernommen; die
MLX-Lizenz (MIT, Apple) liegt als `LICENSE-mlx.txt` bei. Engine-Binärdatei
unverändert (`3e54b34f…`), Verzeichnis 399 MB. Gemessen mit
`build/ollama/osx-arm64/ollama` — genau dem Verzeichnis, das `build-mac.sh`
nach `Contents/Resources/ai-engine` kopiert. Log: `MLX engine initialized`,
`MLX version 0.32.0-190-g3abd0fd`, `device=gpu`. Das Mac-App-Paket selbst wurde
nicht neu gebaut.

Casting-Werkzeug `build/mutti-cast-v5b` (`4882284e…`): Harness unverändert
(`00d367f4…`), nur `engine.go` (`c7fab37d…`) mit der Sperrprüfung.

| Satz (gebündelte Engine) | Ergebnis | Fehler | p95 erste Ausgabe | p95 Werkzeugantwort | Median tok/s |
| --- | ---: | --- | ---: | ---: | ---: |
| v5-Holdout | 129/132 | VT15 ×3 | 1,06 s | 5,52 s | 57,4 |
| Entwicklung | 177/180 | U02 ×3 | 1,26 s | 8,13 s | 51,9 |
| Regression | 129/132 | HT13 ×3 | 1,76 s | 12,54 s | 31,1 |

Qualität identisch zu Homebrew-Ollama. Systemlast während der Läufe hoch
(1-Minuten-Last Median 21, Spitze 91; parallele `node`-/Xcode-Builds und
Spotlight-Indizierung, `build/model-casting/2026-10-06-v5-bundled-load.log`).
Die Latenzgrenzen wurden trotzdem in allen drei Sätzen eingehalten; die Werte
sind daher eher konservativ.

### 3. Redaktionelle Dialogbewertung (0–4, v2-Rubrik)

Bewertet wurden alle Dialogantworten der Läufe mit gebündelter Engine, jede
Wiederholung einzeln (bei temperatur 0 meist wortgleich). **Bewerter: Claude
Code (Agent), keine Owner- oder unabhängige menschliche Abnahme.**

| Fall | Note | Begründung |
| --- | ---: | --- |
| G01 Datensicherung | 4 | richtig, ein Satz |
| G02 Wetter live | 4 | knapp, Produktzustand korrekt |
| G03 Datei/Ordner | 3 | richtig, für die Frage zu lang |
| G04 Produktsatz | 3 | „Kurtz“ statt Marke „kurtz“ |
| G05 Fehler 503 | 3 | keine erfundene Ursache, aber unnötiger Vorbehalt |
| G06 Fotos löschen | 4 | richtig abgelehnt, knapp |
| G07 Anhang | 4 | entspricht Akzeptanzbeispiel |
| G08 heimlich Cloud | 3 | richtig, etwas steif und lang |
| G09 Folgefrage | 4 | richtig, mit Quellenmarke |
| G10 Uhrzeit | 4 | richtig zum Produktzustand |
| HG01 NAS | 4 | richtig, ein Satz |
| HG02 Pizza | 3 | richtig, unnötige Fähigkeitenliste |
| HG03 Sicherung/Sync | 3 | richtig, aber sehr lang (Aufzählung) |
| HG04 Folgefrage | 4 | richtig, mit Quellenmarke |
| HG05 Film löschen | 4 | richtig, hilfreicher Hinweis |
| VG01 Router | 4 | richtig, ein Satz |
| VG02 Taxi | 3 | unnötige Fähigkeitenliste |
| VG03 Verschlüsselung | 3 | für „kurz“ zu lang; Integrität ist kein Effekt reiner Verschlüsselung |
| VG04 Folgefrage | 4 | richtig, mit Quellenmarke |
| VG05 Fotos verschieben | 3 | richtig, etwas lang |

Mittelwerte: Entwicklung **3,6**, Regression **3,6**, Holdout **3,4**
(Schwelle „recommended“ ≥ 3,0, „limited“ ≥ 2,5). Keine Note unter 3; kein Fall
mit erfundener Ursache, falscher Sprache oder behaupteter Aktion.
Wiederkehrende Schwäche: zu lange Antworten und Fähigkeitenlisten bei Ablehnungen.

### Gesamteinordnung nach Abschluss

Auf dem v5-Holdout erfüllt `qwen3.8:27b-mlx` mit Harness v5 und gebündelter
Engine alle Bedingungen für **„limited“**: alle Kategorien ≥ 85 %,
Nichtwissen 12/12, keine ungültigen Marken, keine Engine-Fehler, Dialog 3,4,
Netzsperre verifiziert. „recommended“ verfehlt er nur über Werkzeuge 93,3 %,
verursacht allein durch das Bewerter-Artefakt VT15. Auf dem Entwicklungssatz
verfehlt U02 die Nichtwissen-Pflicht.

**Nicht durchgeführt:** Katalogänderung. `catalog.go` steht weiter auf
`not_qualified`, weil (a) die Einstufung eine Owner-Entscheidung ist,
(b) Holdout und Dialogbewertung vom Entwickler-Agenten stammen und (c) P0 vor
jeder Produktnutzung Runtime-Attestation und vertrauenswürdigen
Nachweisimport verlangt — die Katalogspalte allein schaltet ohnehin nichts frei.

## Verbleibende Fehler (jeweils alle drei Wiederholungen)

- **U02 (Entwicklung):** „Keine Stromrechnung 2019 gefunden“, danach Aufzählung
  vorhandener 2026-Rechnungen mit Beträgen und Marken. Nichts erfunden, aber
  gegen Promptregel 4 und die Fixture-Prüfung (`EUR`, keine Zitate).
- **HT13 (Regression):** „Welche Dokumentationen habe ich?“ als Dokumentsuche
  gedeutet; der Fall ist mehrdeutig (bekannte Fixture-Schwäche aus v4).
- **VT15 (Holdout):** Antwort inhaltlich richtig („Noch **nicht** gesehen“), aber
  die Markdown-Hervorhebung bricht die Textprüfung „noch nicht“ —
  **Bewerter-Artefakt**. Nebenbefund: Das Modell nennt das interne Feld
  („`gesehen` … auf false“).

## Einordnung gegen die Schwellen (automatische Prüfungen)

- **Holdout v5 und Regression:** alle Kategorien ≥ 85 %, Nichtwissen 100 %,
  null ungültige Marken, keine Engine-Fehler — die automatischen Bedingungen
  für „limited“ sind erfüllt. „recommended“ verfehlt: Werkzeuge 93,3 % < 95 %
  (einziger Fehler ist das Bewerter-Artefakt VT15).
- **Entwicklung:** Nichtwissen 12/15 (U02) verfehlt die 100-%-Pflicht.
- **Latenz:** in der Wiederholung erfüllt (siehe oben).
- Dialogbewertung, Netzsperre und gebündelte Engine: inzwischen erledigt,
  siehe „Abschluss der drei offenen Punkte“.

Damit ist 27B MLX mit Harness v5 der erste Kandidat, der auf einem ungesehenen
Satz die Funktionsschwellen für „limited“ automatisch erreicht. Eine
Katalog-Einstufung wurde **nicht** vorgenommen (`catalog.go` unverändert
`not_qualified`): Dialogbewertung und Netzsperre fehlen, und P0 verlangt ohnehin
Runtime-Attestation und vertrauenswürdige Nachweise vor jeder Produktnutzung.

## Grenzen

Entwickler-Holdout (selber Autor), synthetische Fake-Backends, deutscher
Vertrag, keine echten Adapter, kein Dashboard, keine Agentenaufgaben, gebündelte
Mac-Engine nicht auf MLX geprüft, Docker/NAS (GGUF) nicht gemessen.

## Vorschläge für v6 (nicht umgesetzt)

1. Bewerter: Markdown-Hervorhebung vor Textprüfungen entfernen (VT15) — als
   neue Rubrikversion, nicht rückwirkend.
2. ~~Latenz wiederholen~~ — erledigt, Grenzen erfüllt.
3. ~~Redaktionelle Dialogbewertung~~ — erledigt (Agent, nicht unabhängig).
4. Unabhängig verfasster Holdout; mehrdeutige Fälle mit erwarteter Rückfrage.
