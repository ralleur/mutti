# Casting v3 – Ergänzung zur eingefrorenen Bewertung

Festgelegt am 06.10.2026 **vor** der ersten v3-Antwort. Es gelten alle Regeln
und Schwellen aus der [v2-Rubrik](casting-v2-rubric.md). Die v2-Ergebnisse
bleiben unverändert gültig und werden nicht umgewertet.

Begründete, begrenzte Änderungen nach Auswertung von v2:

1. **Promptvertrag `mutti-assistant-v3`:** Laufzeitfilter zusätzlich als
   `runtime_below_minutes` (exklusive Grenze); Sekundenbeispiele präzisiert.
2. **Harness-Wächter:** Behauptet eine Antwort ohne jeden Werkzeugaufruf etwas
   über private Daten („nicht gefunden“, „habe … gesucht“, „keinen Zugriff auf
   deine Unterlagen“), wird der Entwurf zurückgezogen (`reset`) und das Modell
   einmal aufgefordert, zuerst das Werkzeug zu nutzen. Der Nutzer sieht nur die
   geprüfte Antwort. Der Lauf protokolliert `guarded`.
3. **Marken:** eine nackte, vom Server vergebene Marke („ist Q1.“) wird wie `(Q1)`
   normalisiert; Quartalsangaben wie „Q1 2026“ bleiben unverändert.
4. **Fixture `model-casting-v3`** (abgeleitet aus v2 durch `casting-v3-fixture.py`):
   T02/T08/T18 akzeptieren die gleichwertige Minutenangabe; T17 verlangt keine
   Rückfrage mehr, weil der Assistent laut Produktvertrag nichts abspielen kann
   (Abspielbehauptungen scheitern weiterhin); U05 darf das Dokumentenarchiv
   nach einem Kontoauszug durchsuchen.

Kandidaten für v3: Qwen 3.6 35B-A3B (bestes v2-Werkzeugergebnis) und
Qwen 3.5 4B (kleines Profil). Qwen 3.5 9B und Gemma 4 E4B werden wegen
schwächerer Dokument-/Werkzeugwerte in v2 nicht weiter verfolgt.
