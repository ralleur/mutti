# Mutti development

Diese Datei ist die gemeinsame Projektvorgabe für Codex und Claude Code.
`CLAUDE.md` importiert sie; Arbeitsregeln nur hier pflegen.

## Einstieg in jede Session

Vor Planung oder Änderungen, auch bei Wiederaufnahme nach einer Übergabe:

1. [Produkt- und Umsetzungsplan](docs/mutti/plan.md) sichten: Produktgrundsatz,
   verbindliche Entscheidungen und integrierte Reihenfolge P0–P4 (Abschnitt 16).
   Die zum Auftrag gehörenden M-/E-/UX-Arbeitspakete und Abnahmekriterien lesen.
2. [Aktuellen Status](docs/mutti/status.md) lesen, insbesondere die aktuelle
   Übergabe, offene Gates und nächsten Schritte. Historische Tagesabschnitte
   nicht als aktuellen Implementierungsstand ausgeben.
3. [Ausgangsbasis](docs/mutti/baseline-2026-10-06.md) und die für den Auftrag
   relevanten [Verträge](docs/mutti/contracts.md) lesen; verlinkte Fach- und
   Prüfdokumente gezielt hinzunehmen. Die datierte Basis ist eine Momentaufnahme;
   neuere Statusnachträge und überprüfter Code können darüber hinausgehen.
4. Branch, HEAD und `git status --short` prüfen. Vorhandene Änderungen von der
   eigenen Arbeit unterscheiden und erhalten. Dokumentierte Aussagen am
   betroffenen Code/Nachweis abgleichen; Widersprüche ausdrücklich festhalten.
5. Zu Beginn kurz benennen: aktueller Stand, passendes Arbeitspaket und nächster
   konkreter Schritt im Nutzerauftrag. Der Plan allein autorisiert weder das
   Abarbeiten aller Folgephasen noch Veröffentlichung oder neue Dienste.

Bei Fortsetzung innerhalb derselben Session nur relevante Änderungen nachlesen;
nach Kontextverlust die aktuelle Übergabe und den Arbeitsbaum erneut prüfen.
Neuere ausdrückliche Nutzerentscheidungen haben Vorrang und werden bei Übernahme
im Plan nachvollziehbar festgehalten.

## Umsetzung und Übergabe dokumentieren

- `docs/mutti/plan.md` beschreibt Ziel, Entscheidungen, Reihenfolge und
  Abnahmekriterien. Bei beschlossenen Umfangs-/Architekturänderungen aktualisieren;
  keine konkurrierenden Gesamtpläne oder agentenspezifischen Aufgabenlisten anlegen.
- `docs/mutti/status.md` ist die aktuelle Übergabe. Nach jedem abgeschlossenen
  Arbeitspaket und vor einer geplanten Unterbrechung oder dem Sessionabschluss
  den Stand aktualisieren, sofern Code, Planung oder Erkenntnisse verändert wurden.
  Bei längerer Arbeit Zwischenstände sichern, nicht erst am Schluss dokumentieren.
- Festhalten: Datum, P-/M-/E-/UX-Bezug, konkrete Änderungen und betroffene Dateien,
  Entscheidungen samt Begründung, ausgeführte Prüfungen mit Ergebnis, offene
  Risiken/Blocker und die nächsten ausführbaren Schritte samt Voraussetzungen.
  Branch/Commit angeben, sofern vorhanden; uncommittete Arbeit ausdrücklich nennen.
- Implementiert, gebaut, synthetisch geprüft, im echten Paket/auf Hardware geprüft
  und freigegeben getrennt ausweisen. Nicht ausgeführte oder fehlgeschlagene Tests
  nennen; fehlende Belege sind kein Erfolg. Bei Teilabschluss verbleibende Arbeit
  und einen konkreten Wiederaufnahmepunkt hinterlassen.
- Umfangreiche Prüfnachweise unter `docs/mutti/evidence/` ablegen und vom Status
  verlinken: Datum, Quellstand, Umgebung, Befehle, Ergebnisse und Grenzen.
  Keine Geheimnisse oder privaten Inhalte aufnehmen; keine Routine-Logkopien
  ohne zusätzlichen Nachweiswert. Kleine Dokumentationsänderungen brauchen nur
  einen knappen Statusnachtrag mit ihrer Prüfung.
- Fach-/Vertragsdokumentation mit geändertem Verhalten abgleichen. Historische
  Belege erhalten und als überholt einordnen, nicht rückwirkend umschreiben.
  Im Abschluss die gepflegten Dokumente und noch offene Gates verlinken.
- Reine Fragen ohne neue Erkenntnisse oder Änderungen erfordern keinen künstlichen
  Statusnachtrag. Chatverlauf und private Agenten-Memory ersetzen keine Repo-Übergabe.

## Bestehende Entwicklungsgrenzen

- Preserve upstream API compatibility, file-specific licenses and contributor history.
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
