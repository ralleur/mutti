# Casting v5 – Protokoll

Festgelegt am 06.10.2026. Regeln und Schwellen wie in der
[v2-Rubrik](casting-v2-rubric.md) und den Bewertungsänderungen der
[v4-Rubrik](casting-v4-rubric.md) (Filterbedeutung, `german`, `interventions`).

**Ziel (Nutzerentscheidung):** Entwicklung ausschließlich für Qwen3.8 27B als
MLX-Variante (`qwen3.8:27b-mlx`). Ein vorgeschaltetes Decider-Modell ist
vorerst nicht Teil des Vorhabens.

## Prüfsätze

| Satz | Datei | Rolle |
| --- | --- | --- |
| Entwicklung | `model-casting-v5-dev.json` | v4-Entwicklungssatz (60 Fälle) mit Prompt v5, sonst unverändert. |
| Regression | `model-casting-v5-regression.json` | v4-Holdout (44 Fälle) mit Prompt v5. **Gesehen**: seine Fehler wurden nach v4 ausgewertet; zählt nicht als Holdout. |
| Holdout | `model-casting-v5-holdout.json` | 44 neue Fälle, neue synthetische Daten (`casting-v5-holdout.py`). |

Beide abgeleiteten Sätze erzeugt `casting-v5-dev.py`.

## Holdout-Freeze

Erstellt **vor** jeder v5-Code-Änderung (harness.go unverändert seit dem
v4-Freeze, SHA-256 `a1e6b573…`). SHA-256 des v5-Holdouts:
`04c15c20a790c451ac5b323f4636da85e3d756751c0567571d339ca62b4fca3a`.
Nach dem ersten Holdout-Lauf keine Anpassung mehr an Harness, Prompt oder
Holdout. Grenze wie in v4: vom selben Agenten verfasster Entwickler-Holdout.
Bekannte Lücken, die der Autor beim Schreiben kannte (z. B. Formulierung
„Favoritenliste“, die das Favoriten-Muster des Harness nicht erkennt), werden in
v5 bewusst **nicht** geschlossen, um den Holdout nicht zu verzerren.

## Harness-Freeze vor Regression und Holdout

Nach einer Entwicklungsiteration (27B MLX 177/180) eingefroren, bevor ein Modell
den v5-Holdout gesehen hat. SHA-256:

| Datei | SHA-256 |
| --- | --- |
| `mutti/hub/harness.go` | `00d367f4323a8e65927a809212f19485c0bb147348302dbf5fd44f3822c0ff44` |
| `mutti/hub/tools.go` | `469d18a8095d6ba427a5f84c0958c99991fbe3e7198311a15542170e37806592` (wie v4) |
| `mutti/hub/cast.go` | `da332258da50b7fda97dba52ecc69253e8a7be4965fe14b0aca1a00370aceeb5` (wie v4) |
| `model-casting-v5-dev.json` | `2325486979a3a4ac504816da01db60b616eb7d8bf2c6caaa94c5412e0702c768` |
| `model-casting-v5-regression.json` | `09badff887516829119ef3f3b219a4cb8e5b85cd7068a172d4f92920210f0917` |
| `model-casting-v5-holdout.json` | `04c15c20a790c451ac5b323f4636da85e3d756751c0567571d339ca62b4fca3a` (unverändert) |
| `build/mutti-cast-v5` | `a2b7e7cad81ceeda4736ff85fd9ef34ad1bb38717d244cec62d899503fb328fe` |
