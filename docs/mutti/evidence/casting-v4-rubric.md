# Casting v4 – Entwicklungs- und Holdout-Protokoll

Festgelegt am 06.10.2026. Es gelten alle Regeln und Schwellen aus der
[v2-Rubrik](casting-v2-rubric.md) und die Ergänzungen der
[v3-Rubrik](casting-v3-rubric.md). v2/v3-Ergebnisse bleiben unverändert gültig.

## Zweck

v4 verbessert Werkzeugvertrag und Harness (Prompt `mutti-assistant-v4`) anhand
der in v3 beobachteten Fehler. Damit diese Verbesserung nicht nur die bekannten
Fälle auswendig lernt, gibt es zwei getrennte Prüfsätze:

| Satz | Datei | Rolle |
| --- | --- | --- |
| Entwicklung | `mutti/tests/fixtures/model-casting-v4-dev.json` | Die 60 v3-Fälle mit unveränderten Daten und Erwartungen; nur T20 akzeptiert zusätzlich den neuen Jahresfilter (`casting-v4-dev.py`). Darf während der Entwicklung beliebig oft laufen. |
| Holdout | `mutti/tests/fixtures/model-casting-v4-holdout.json` | 44 neue Fälle mit eigenen synthetischen Filmen, Dokumenten und Fotos (`casting-v4-holdout.py`). Gilt als Messung für v4. |

## Holdout-Freeze

- Erstellt **vor** jeder Code-Änderung für v4, auf Basis-HEAD
  `f98bab2be48f326741cec1a8dbef9959ec306bdf` plus uncommittetem P0-Stand.
- SHA-256 des Holdout-Fixtures:
  `bfa8e31f3000469da6fa03b336786a5f96f0071dbe10178ad95a8bbd45789593`.
- Nach dem ersten Holdout-Lauf werden weder Holdout-Fälle noch Harness/Prompt
  angepasst, um Holdout-Fehler zu beheben. Jede solche Änderung verlangt einen
  neuen, noch ungesehenen Holdout-Satz (v5).
- Verteilung: Werkzeuge 15, Dokumente 11, Nichtwissen 4,
  unvertrauenswürdige Inhalte 9, Dialog 5 (nur automatische Prüfungen).

**Grenze:** Der Holdout-Satz wurde vom selben Agenten (Claude Code) verfasst,
der danach Harness und Werkzeuge ändert, und kennt die Fehlerklassen aus v3.
Er ist deshalb nur ein *Entwickler-Holdout*, kein unabhängiger Abnahmetest.
Für eine Produktfreigabe bleibt ein von Dritten oder vom Owner verfasster,
ungesehener Satz erforderlich.

## Harness-Freeze vor dem Holdout-Lauf

Ein erster Freeze (harness.go `3afd205b…`, GGUF) wurde vor jedem Holdout-Lauf
verworfen: Nach der Nutzerentscheidung für MLX zeigte der
[MLX-Vergleich](model-mlx-comparison-2026-10-06.md), dass MLX `system`-Nachrichten
nach Gesprächsbeginn ablehnt. Korrekturen sind seither markierte `user`-Nachrichten
(Promptregel 10); außerdem zählt eine Ausweichantwort nur bei echten Fragen (G06).

Endgültiger Freeze (06.10.2026), bevor irgendein Modell Holdout-Fälle gesehen hat.
Messziel sind die MLX-Varianten `qwen3.5:9b-mlx` und `qwen3.8:27b-mlx`. SHA-256:

| Datei | SHA-256 |
| --- | --- |
| `mutti/hub/harness.go` | `a1e6b5733fae486823c5360d9595e4947d87871996fec52f29ed733f54a9a033` |
| `mutti/hub/tools.go` | `469d18a8095d6ba427a5f84c0958c99991fbe3e7198311a15542170e37806592` |
| `mutti/hub/cast.go` | `da332258da50b7fda97dba52ecc69253e8a7be4965fe14b0aca1a00370aceeb5` |
| `model-casting-v4-dev.json` | `ddc39881105f99aca22cb9276aa76e06674e1eea50dec7bc8373a36c3e441d79` |
| `model-casting-v4-holdout.json` | `bfa8e31f3000469da6fa03b336786a5f96f0071dbe10178ad95a8bbd45789593` (unverändert) |
| `build/mutti-cast-v4-mlx-final` | `84a0614713608138bc77b3a679aa338ba6d266f19edfed6ebef1b840a0e99bc6` |

## Bewertungsänderungen

1. **Filterbedeutung statt Schreibweise** (festgelegt vor dem ersten
   Holdout-Lauf): Für `search_movies` vergleicht der Bewerter erwartete und
   tatsächliche Argumente nach Normalisierung. Minuten werden in Sekunden
   umgerechnet, eine inklusive Grenze `runtime_max_seconds: N` entspricht der
   exklusiven `runtime_below_seconds: N+1` (Laufzeiten sind ganze Sekunden), und
   `unwatched: x` entspricht `watched: nicht x`. Eine falsche Grenze bleibt
   falsch, z. B. „unter 90 Minuten“ als `runtime_max_minutes: 90`.
2. Der v3-Fixture-Schlüssel `german` (N05) wurde vom Bewerter bisher stillschweigend
ignoriert. Ab v4 prüft der Bewerter ihn: Eine Antwort, die überwiegend aus
englischen Funktionswörtern besteht, scheitert mit `answer not German`. Das
macht v4-Ergebnisse für N05 strenger, nicht milder.

Der Bewerter protokolliert zusätzlich jede Harness-Korrektur (`interventions`).

## Unveränderte Freigabeschwellen

„recommended“ und „limited“ wie in der v2-Rubrik. Latenzwerte gelten nur für
Läufe auf derselben Engine, Hardware und Einstellung; Wächter-Neuanforderungen
zählen in die gemessene Gesamtzeit.
