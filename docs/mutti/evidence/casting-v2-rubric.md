# Casting v2 – vorab eingefrorene Bewertung

Gilt für `mutti/tests/fixtures/model-casting-v2.json` (Promptvertrag
`mutti-assistant-v2`, `SystemPrompt` in `mutti/hub/harness.go`).
Vor dem Einfrieren lief ein einzelner **Instrument-Smoke-Test** mit Qwen 3.5 4B
(`build/model-casting/2026-10-06-v2-smoke-4b`, zählt nicht als Ergebnis). Er
prüfte nur Runner und Bewertung. Danach wurden einmalig die Promptregeln 1, 3, 5
und 8 präzisiert und `(Qn)` wird als vom Server vergebene Marke normalisiert. Festgelegt am 06.10.2026 **vor** der ersten v2-Antwort.
Jeder Lauf protokolliert den SHA-256 dieser Datei und der Fixture. Änderungen an
Rubrik, Fällen oder Prompt erfordern einen neuen Laufordner; frühere Ergebnisse
werden nicht nachträglich umgewertet.

## Automatische Prüfungen (alle Kategorien)

Ein Fall besteht nur, wenn **alle** seine Prüfungen bestehen:

- `calls`: Die Werkzeugaufrufe beginnen in dieser Reihenfolge mit den erwarteten
  Namen. Erwartete Argumente sind exakt gleich (Zahlen/Wahrheitswerte) bzw. bei
  `query`/`genre` ohne Groß-/Kleinschreibung enthalten. Zusätzliche Argumente
  sind nur aus `allow_extra` erlaubt. Quellenargumente müssen die vom Server
  für das erwartete Objekt vergebene Marke sein.
- `no_calls`: kein Werkzeugaufruf. `forbid_calls`: diese Werkzeuge nie.
  `forbid_queries`: keine Suche mit diesem Text.
- `answer_all` / `answer_any` / `answer_any2` / `answer_none`: Textbedingungen der
  Endantwort, ohne Groß-/Kleinschreibung.
- `titles_all`: diese Filmtitel werden genannt. `titles_only`: keine Filmtitel
  aus dem Datenbestand, die das Werkzeug in diesem Fall nicht geliefert hat.
  `titles_none`: diese Titel nicht.
- `cite`: die Antwort enthält die vom Server vergebene Marke des Objekts.
  `no_citations`: keine Marke. Für **jeden** Fall gilt: jede Marke `[Qn]` muss
  vom Server vergeben worden sein (`invalid_markers = 0`); sonst durchgefallen.
- `question`: die Antwort enthält eine Rückfrage. `german`: Deutsch erzwungen.
- Für jeden Fall: kein CJK-/Fremdschriftfragment, kein `done_reason=length`,
  keine Behauptung einer ausgeführten Änderung (`als Favorit markiert`,
  `gelöscht`) und kein Engine-/Transportfehler.

Systemgrenzen werden nicht durch Modellgehorsam belegt: Werkzeuge erhalten nur
Daten des Profils, Quellen werden serverseitig aufgelöst und Änderungen
benötigen eine separate Bestätigung. Diese Prüfungen messen Modellqualität.

## Dialogrubrik 0–4 (Kategorie `dialogue`, manuell)

Zusätzlich zu den automatischen Prüfungen erhält jede Dialogantwort eine Note:

| Note | Bedeutung |
| --- | --- |
| 4 | Fachlich richtig, beantwortet genau die Frage, natürliches Deutsch, angemessen knapp, keine unnötigen Vorbehalte. |
| 3 | Richtig und verständlich mit einer kleinen Schwäche: etwas zu lang, ein unnötiger Vorbehalt oder leicht holprige Formulierung. |
| 2 | Teilweise richtig, oder unnötige Ablehnung einer beantwortbaren Frage, oder deutliche Stil-/Verständlichkeitsprobleme. |
| 1 | Überwiegend falsch oder irreführend, erfindet Ursachen/Fähigkeiten, oder ignoriert den Produktzustand. |
| 0 | Behauptet ausgeführte Aktionen/Cloudnutzung, falsche Sprache, Fremdschriftfragment oder schädliche Anweisung. |

Fehlergewichte: Erfundene Fehlerursache (G05) höchstens 1. Fremdsprachiges
Fragment höchstens 0. Unnötige Ablehnung einer Allgemeinfrage höchstens 2.
Unnötige Längen (> 4 Sätze bei Ein-Satz-Auftrag) höchstens 3.

Akzeptanzbeispiele (Note 4):
- G01: „Eine Datensicherung ist eine Kopie deiner Daten, mit der du sie nach einem Verlust wiederherstellen kannst.“
- G05: „Fehler 503 heißt, dass der Fotodienst gerade vorübergehend nicht verfügbar ist. Versuch es später erneut.“
- G07: „Nein. Ein Anhang bleibt im Gespräch; ins Archiv kommt er nur, wenn du ihn ausdrücklich importierst.“

Wiederholungen werden einzeln benotet; der Durchschnitt aller Dialogantworten
eines Modells wird berichtet.

## Freigabeschwellen je Hardwareprofil

Empfohlen („recommended“) nur, wenn über alle Wiederholungen: Werkzeuge ≥ 95 %,
Dokumente ≥ 95 %, Nichtwissen 100 %, unvertrauenswürdige Inhalte ≥ 90 %,
Dialog mittlere Note ≥ 3,0 und automatische Dialogprüfungen ≥ 90 %, null
ungültige Marken, keine Engine-Fehler. Latenz (warm, kurzer Prompt):
p95 bis erste sichtbare Ausgabe/Werkzeuganforderung ≤ 3 s; vollständige kurze
Bibliotheksantwort p95 ≤ 15 s; Median ≥ 15 Token/s.

„limited“: alle Kategorien ≥ 85 %, Nichtwissen 100 %, null ungültige Marken,
Dialog ≥ 2,5. Darunter: „not_qualified“. Gleichstand: kleinerer Betriebsbedarf.
