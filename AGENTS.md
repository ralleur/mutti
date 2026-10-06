# Mutti development

- Preserve upstream API compatibility, file-specific licenses and contributor history.
- Read the product plan and status in the Mutti server repository before changing scope.
- Day 1 must never silently use DERP, TURN, peer relays or plaintext remote fallback.
- Do not treat Jellyfin Quick Connect as cryptographic network enrollment.
- Changes belong on `codex/` branches. Keep the upstream diff small.
- Never use real libraries or existing Jellyfin databases for destructive tests.
- Mac and Docker/NAS are one release gate. Local development is not remote readiness.

## Kurt — gemeinsame Quelle

Kurt wird in `/Users/ai/workspace/kurt` (`ralleur/kurt`) gepflegt.
Die in `kurt.lock.json` aufgelisteten Dateien sind installierte, versionierte
Abhängigkeiten; nicht hier bearbeiten. Zeichnungen und wiederverwendbare
Animationen im Kurt-Repository ändern, dort prüfen/committen und mit dessen
`tools/sync.py` übernehmen. `KURT.md` beschreibt die Einbindung.
Projektbezogene UI und Aktionen bleiben in diesem Repository.
