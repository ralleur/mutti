# Lokale Sicherung, Rechte und Wiederanlauf

Entwicklungsstand vom 5. Oktober 2026. Unterstützt ist der paketierte
Jellyfin-12.1-Stand auf Mac arm64 und im lokal geprüften Docker-arm64-Paket.
Dies ersetzt weder eine Kopie der Originalmedien noch ein externes Sicherungsziel.

## Benutzung

In Mutti als Besitzer anmelden, „Speicher“ öffnen und „Sicherung erstellen“
wählen. Das Online-Backup erstellt konsistente Datenbankkopien; Mutti ergänzt
den qualifizierten Intro-Skipper-Zustand und SHA-256-Prüfsummen. Eine erstellte
Sicherung ist zunächst **noch nicht durch Wiederherstellung geprüft**.

„Prüfen“ benötigt den Besitzerzugang zum Zeitpunkt der Sicherung. Mutti baut eine
getrennte Instanz auf, prüft Anmeldung, Bibliotheken, Benutzer, Wiedergabedaten,
Dateipfade und Pluginzustand und verwirft anschließend ausschließlich diese
erzeugte Prüfkopie. Erst danach erscheint der Prüfzeitpunkt.

„Wiederherstellen“ verlangt dieselben Prüfungen und eine ausdrückliche Bestätigung.
Erst nach Erfolg wird die aktive Instanz gewechselt; der vorherige Datenordner
bleibt erhalten. Änderungen seit der Sicherung werden nicht zusammengeführt.
Alte Sitzungen und Gerätefreigaben werden nicht übernommen: neu anmelden und
Geräte erneut koppeln. Die aktuelle Besitzerberechtigung wird vor dem Wechsel
erneut geprüft. Ein falsches Kennwort oder eine defekte Sicherung schaltet nichts um.

Sicherungen liegen als private Verzeichnisse unter `<Mutti-Datenordner>/backups`.
Jede enthält `backup.json`, `library.zip` und gegebenenfalls `intro/`.
Zum Schutz vor Plattenverlust den **gesamten jeweiligen Sicherungsordner** auf
separaten Speicher kopieren. Die Dateien enthalten private Konfiguration und
Passworthashes: geschützt aufbewahren. Ein komfortabler Import fremder
Sicherungsorte, automatische Aufbewahrung und Sicherungspläne sind noch offen.

## Rechte und Betrieb

Profilrechte werden durch Jellyfin geprüft, nicht nur in der GUI ausgeblendet.
Mutti Connect prüft Profilbindung und vollständige Benutzer-Policy fortlaufend.
Gerätesperre, Profilsperre oder Policyänderung beendet alte Verbindungen; nach
einer Rechteänderung kann ein weiterhin erlaubtes Gerät mit neuen Rechten neu
verbinden. Bereits lokal gepufferte Filmbytes lassen sich nicht zurückrufen.

Ein abgestürzter verwalteter Jellyfin-Prozess wird höchstens dreimal innerhalb
von fünf Minuten mit Abstand neu gestartet. Eine Absturzschleife erzeugt eine
Fehlermeldung statt endloser Neustarts. Connect wird vor Wiederanlauf angehalten
und erst nach Bereitschaft und abgeschlossener Einrichtung wieder gestartet.
Aktive Sicherungs-/Importarbeiten werden nicht durch parallele Wiederanläufe
überholt. Unvollendete Wartungsaufträge werden nach Managerneustart als
unterbrochen angezeigt, niemals als erfolgreich.

## Prüfumfang und verbleibende Grenzen

- Synthetische Mac-Migration einschließlich Intro Skipper, Sammlung, Resume,
  Favoriten, Probe und Restore; Originalquelle und vorherige Zieldaten erhalten.
- Tatsächliches Docker-Paket über Verwaltungs-API: Owner/Viewer-Grenzen,
  Sicherung, falsches Kennwort, Probe, Restore, ungültige alte Sitzung sowie
  Prozessabsturz und Wiederanlauf.
- Browser: echte Probe gestartet und beendet, laufender Zustand und gesperrte
  Parallelaktionen, Wiederherstellungsdialog bei 390 × 844 px sowie Escape/Fokus.
- Transport: aktive Streams bei Geräte-/Profilsperre und Bibliotheksänderung
  geschlossen; neue Rechte greifen beim Wiederverbinden. Verwaiste Datenkanäle
  nach nativem Neustart blockieren den Widerruf nicht mehr. Go-Race-Detector
  und tatsächlicher kurtz-Build 78 geprüft.

Nur Wiederherstellung auf demselben qualifizierten Paket mit erreichbaren
Medienpfaden ist abgenommen. Vollständiger Rechnerverlust, neue Pfadzuordnung,
Versionswechsel/Update-Rollback, große Bibliotheken, Stromverlust während
Umschaltung und physische NAS-Systeme bleiben offen. Keine allgemeine
Backup- oder Releasefreigabe ableiten.
