# Foundation – lokaler Abnahmebeleg, 05.10.2026

Mac Studio/Apple Silicon, macOS, Go 1.27.1, .NET 10.0.401, Xcode 27,
Node 26.7.0; Docker Linux arm64. Ausschließlich eigene synthetische Bestände
für Migration, Rechteentzug, Prozessabsturz und Restore. Vorhandene Installationen,
laufende Dienste und Änderungen anderer Arbeiten wurden erhalten.

## Ergebnisse

| Prüfung | Ergebnis / Grenze |
| --- | --- |
| Go-Wartung, Import, Wiederanlauf | Race-Detector und `go vet` bestanden; beschädigte/unerlaubte Archive, falscher Besitzerzugang, konkurrierende und unterbrochene Aufträge geprüft |
| Tatsächlicher Mac-Import + Restore | 86,84 s; Logins, Benutzerrechte, Bibliotheks- und Sammlungsidentität, Playlist, Resume, Favoriten, Intro Skipper, erhaltene Quelle und vorheriger Datenstand geprüft |
| Docker-Verwaltung des finalen Images | 39 Prüfungen bestanden: Owner/Viewer, Host/Origin, begrenzte Eingaben, Profile/Bibliotheken, Sicherung, falsches Kennwort, Probe, Restore, alte Sitzungen, Absturz/Wiederanlauf |
| .NET-Grenzen | 12 Tests bestanden |
| Web | TypeScript, ESLint/Stylelint der Änderungen und Produktionsbuild bestanden; bekannte Bundlegrößenwarnungen bleiben |
| Eigene Verwaltung im Browser | Reale Rechteänderungen und Restore-Probe; Warte-/Fehlerzustände, bestätigter Abschluss, keine Parallelaktion, 390-px-Dialog und Fokus/Escape geprüft |
| Mac-Hülle | Build und fünf Swift-Tests bestanden; Wiederanlaufmeldung wird auch in Hülle und Einrichtungsansicht angezeigt |
| Connect | Race-Detector und `go vet`; Geräte-, Profil-, Bibliotheks- und Wiederverbindungsfälle bestanden |
| Echter Medienpfad Mac + Docker | Verschlüsselte bytegenaue Range-206-Antwort, reales HLS-Transcoding mit Playlist/Segment, WebSocket, Zugriff nach Widerruf verweigert; 9,14 s Mac / 16,58 s Docker |
| Native Medienoberfläche | Bestehende kurtz-App, keine Demo; synthetische Kopplung, Freigabe, Profil, Filmdetail, Wiedergabe und 15-Sekunden-Sprung sichtbar durchlaufen (Build 76/77) |
| Finaler kurtz-Build 78 | Automatischer Profileinstieg; App beenden/starten; echte Bibliothek erneut geladen; Widerruf nach 0,019 s abgeschlossen; Library zeigt anschließend verständliche Freigabemeldung |
| Native Builds | Universeller Catalyst-Build 78 mit bestehender Apple-Entwicklungssignatur; iOS-Simulator arm64 gebaut; Signaturen streng verifiziert |
| Native Logiktests | 11 PlaybackCore + 53 KurtzLogic bestanden |
| Paketstarter Mac | Direkt aus dem kopierten Paket gestartet; Neu einrichten führt sichtbar in den echten Assistenten; Ctrl-C beendet alle vier eigenen Listener, Daten bleiben erhalten |
| Docker-Archiv | Exportiertes Archiv erneut mit `docker load` eingelesen; exakt dieselbe Image-ID wie beim Pakettest |
| Rechteprüfung | Normale Prüfung: 78 Inputs bestanden. Releaseprüfung: zwei bestehende Blocker; kein Release/Upload |

### Korrigierter Fehler beim Widerruf

Im tatsächlichen nativen Neustarttest blieb der Widerruf an einer verwaisten
SCTP-Verbindung hängen. `DataChannel.Close` löst einen geordneten Reset aus;
ohne Gegenstelle blieb der Leser darunter blockiert, während `yamux.Close`
auf dessen Ende wartete. Der Transport setzt beim Schließen jetzt zwingend
Lese-/Schreibfristen und verhindert das spätere Zurücksetzen einer geschlossenen
Verbindung. Außerdem schließt er Sitzungen außerhalb der globalen Sperre.

Regression: blockiertes Lesen und Schreiben ohne Peer-Antwort aufwecken,
geschlossene Frist nicht wieder öffnen; realen Tunnel nach Peer-Abbruch neu
aufbauen und anschließend begrenzt widerrufen. Beide Tests und der tatsächliche
kurtz-Build 78 bestehen. Keine Relay-/Klartext-Ausweichroute hinzugefügt.

## Herkunft und Rohbelege

Die Pakete sind bewusst als `local-development`, `releaseReady: false` markiert.
Server-HEAD `f07d690a057a62a33780fdb3d989f99f0b9380ef`,
Web-HEAD `6bb66379eec53599ee7a15fccdbc1109b66adb0d` plus dokumentierte
Arbeitskopieänderungen. Der native Transport-Build 78 enthält die lokal
geprüfte Reparatur aus dieser Arbeitskopie. Der bestehende Release-Pin
`0b6cbcf58c9d4da95079dbe3c3b80d31e378b275` wurde **nicht** als Beleg für
diesen abweichenden Binärstand ausgegeben. Die lokale Build-Ausnahme
`MUTTI_ALLOW_DIRTY=1` ist in der Provenienz vermerkt; Source-/Binärhashes liegen bei.

Bereinigte Protokolle und Manifest:
`/Users/ai/workspace/mutti/build/testpakete/mutti-pruefnachweise-2026-10-05`.
Die vollständigen internen Buildlogs verbleiben in `/tmp/mutti-autonomy-*.log`;
maßgebliche Testausgaben sind zusätzlich in den dauerhaften Belegordner kopiert.
Native synthetische Fixture:
`/Users/ai/workspace/mutti/build/native-client-2026-10-05-v4`.
Deren `private.json` gehört nicht in Testpakete oder eingecheckte Belege.

## Nicht abgenommen

WAN, physisches NAS, zweiter Mac, reales iPhone/iPad, Stromverlust bei Umschaltung,
Langzeitbetrieb/große Bestände, vollständiger Rechnerverlust, Update mit
Datenmigration und vollständige Accessibility-/DE/EN-Abnahme. Wiederherstellung
gilt nur für dasselbe qualifizierte Paket und erreichbare Medienpfade. Sicherung
der Originalmedien, externe Sicherungsziele und automatische Aufbewahrung fehlen.
KI/Fotos/Dokumente haben noch keine vollständige Produkt-/Clientintegration.

Die allgemeine Mutti-Fertigstellung und die Release-Gates bleiben offen.
