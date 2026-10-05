# Umsetzungsstand

Stand: 5. Oktober 2026. **Lokale Entwicklungsvorschau; der Gesamtplan ist nicht abgeschlossen.**

Die Forks `ralleur/mutti` und `ralleur/mutti-web` behalten ihre vollständige
Jellyfin-Historie. Die Produktbranches beginnen beim zusammenpassenden stabilen
Stand v12.1. Die Umsetzung liegt zunächst auf `codex/mutti-foundation`; `main`
ist die Review-Basis. Der Web-Commit ist im Komponentenmanifest festgelegt.

| Etappe | Ergebnis | Noch offen |
| --- | --- | --- |
| M0 | Unveränderte Mac-Server- und Web-Referenz gebaut. Zwei echte tsnet-Knoten übertragen lokal verschlüsselt Daten mit ausschließlich STUN; kein DERP-Server läuft. | Getrennte Internetanschlüsse, gesperrtes UDP, CGNAT/IPv6, Netzwechsel; verbindliche Transport-/Control-Entscheidung. |
| M1 | Eigene Forks, Komponenten-Pins, Mac- und Docker-Builds, Entwicklungsanleitung, Sicherheitsregeln und CI implementiert. Lokale API-/Videodatenstrecke auf beiden Paketen bestanden. | Vollständiges Quell-/Lizenzinventar für Distribution, weitere Architekturen und vollständige Wiedergabeabnahme. |
| M2 | Eigene Mutti-Vektormarke, Sora, kurtz-Farben, Web-Assistent; native Mac-Hülle mit Serverstart, Status, Ordnerdialog-Brücke und getrennten Daten; nicht privilegiertes Docker-Paket mit schreibgeschützten Medien. | Native Ordnerauswahl durch alle Dialogschritte, Screenreader/Hellmodus vollständig, echte NAS-Installation; einfache sichere Verwaltung von einem zweiten Gerät. Der derzeitige NAS-SSH-Tunnel ist nur ein Entwicklerweg. |
| M2b | Vollständiger Jellyfin-Import nach Owner-Test neu als Anforderung aufgenommen; API-/Backup-Grenze geprüft. | Erstwahl, lokale Erkennung, Quell-Anmeldung, konsistente Datenübernahme, Pfad-/Versionsprüfung und Abnahme. Archivtransport oder Exporthelfer noch zu entscheiden. |
| M3 | QR-Einladung, TLS-Geräteidentität, bestätigte Profilfreigabe, Keychain-Integration und laufender Widerruf implementiert und lokal geprüft. | Wiederherstellung, reale Geräte und vollständige Ablauf-/Bedienabnahme. |
| M4 | Direkter verschlüsselter Transport und Vermittlungsdienst als Teststand implementiert; siehe unten. Kein Relay. | Öffentlicher Testbetrieb, WAN-Matrix und Wiedergabe bei Netzwechseln. |
| M5 | Lokale Sicherheits- und Integrationstests vorhanden. | Backup/Restore, Upgrade, Langzeittests, reale iOS-/tvOS-Geräte, NAS und zwei echte Anschlüsse. |
| M6 | Lokale Mac-App und Docker-Image verfügbar. | Gemeinsame vollständige Abnahme, Developer-ID/Notarisierung, Quellpakete und freigegebenes Release. |

## Tatsächlich geprüft

- .NET 10.0.401, Node 26.7.0, Xcode 27.0, Go 1.27.1; Linux arm64 im lokalen Docker.
- Unveränderte Jellyfin-v12.1-Referenz für den Mac-Server und Web gebaut.
- Mutti-Server: zwölf Tests gegen fremde Hosts/Origins, gefälschte Forwarded-Header,
  nicht lokale Zugriffe und ungültige Preview-Konfiguration bestanden.
- Mutti Web: Produktionsbuild und TypeScript; ESLint für geänderte Abläufe,
  Stylelint für geänderte Styles. Web-CI des gepinnten Stands erfolgreich.
  Auch die eigenen Server-, Mac- und Linux-amd64-Container-CI-Prüfungen bestanden.
- Regionale Sprachauswahl wurde nach einem echten WKWebView-Sichttest korrigiert
  und gegen `de-DE`, `de_AT`, `en-gb` und ungültige Sprachcodes getestet.
- Mac und Docker: jeweils frische Daten, Besitzerzugang, nicht administratives
  Wiedergabeprofil, synthetisches 12-Sekunden-Video importiert, Quick Connect erst
  nach Besitzerfreigabe, Admin-Endpunkt verweigert, HTTP-Range `206` bytegenau
  geprüft, nach Geräteentfernung weitere authentifizierte Anfragen verweigert.
- Mac-App tatsächlich gestartet, Assistent sichtbar; App-Beenden entfernt den
  lokalen Server-Listener. Lokale Ad-hoc-Signatur verifiziert. Keine Notarisierung.
  Der finale Mac-Build stammt aus sauberen Server-/Web-Commits; die Sprachvorgabe
  wurde erneut im echten Fenster geprüft. Docker-Neustart erhält Serveridentität
  und abgeschlossene Einrichtung; der temporäre Testcontainer wurde entfernt.
- `directlab`: echter Userspace-WireGuard-Datenweg und Disco-Endpunkt, weder DERP
  noch Peer-Relay. Ein Rechner; keine Aussage über NAT-Erfolgsquoten im Internet.

Die Testberichte unter `docs/mutti/evidence/` enthalten keine Testpasswörter,
Zugriffstokens, realen Medien oder Benutzerkonten. Vollständige Build-/Serverlogs
bleiben lokal unter `build/` bzw. im temporären Testverzeichnis.

## QR-Kopplung und direkter Transport

Auf den Folgeauftrag „entwickle weiter inkl qr kopplung und direkter fernzugriff
 dann teste ich“ wurde M3/M4 weiter implementiert; der reale Netztest blockiert
 die Implementierung nicht mehr. Details und Testanleitung: [connect.md](connect.md).

Der neue gemeinsame Go-Dienst enthält accountlose Signalisierung, ausschließlich
 direkten Pion-Datenkanal, TLS-1.3-Identitätsbindung, kurzlebige Einladungen,
 Besitzerfreigabe pro Geräteschlüssel, Profilbindung und laufenden Widerruf.
 Mutti Mac und Docker starten ihn; kurtz erhält Keychain-Speicherung, QR-Scanner,
 Linkannahme und einen gemeinsamen lokalen Gateway für sämtliche Clientwege.

Lokal geprüft: Race-Detector und statische Go-Analyse; echte Mutti-Instanz mit
 synthetischem Video, Gerätefreigabe, nicht administrativem Profil, Range-206-
 Bytes, Jellyfin-WebSocket und Sperre nach erneutem Verbindungsaufbau. Die
 Tests verwenden Loopback-ICE, keinen WAN-Nachweis. Derselbe Ablauf besteht auch
 im tatsächlichen Docker-Paket über dessen Geräteverwaltung. Mac-, iOS- und
 tvOS-App-Builds mit eingebettetem Transport sind erfolgreich. Builddetails stehen in der
 Testanleitung. Physische Geräte und getrennte Anschlüsse prüft der Owner.

Ein öffentlich erreichbarer HTTPS-/STUN-Vermittler ist paketiert, aber nicht
 betrieben. Ohne dessen Adresse ist der Test auf das Heimnetz begrenzt. Es wurde
 kein Hosting gebucht, kein Cloudflare-Dienst angelegt und kein Relay aktiviert.
 Day 2 bleibt eine spätere neue Marktprüfung.

## Rückmeldung nach dem ersten Nutzertest (5. Oktober)

Die Mac-Hülle unterscheidet nun Serverbereitschaft und abgeschlossene Einrichtung.
Geräte-Kopplung und Fernzugriff werden erst nach Jellyfins bestätigtem Setup-Abschluss
angeboten; vorher startet auch der Kopplungsdienst nicht. Ein Neustart prüft den
Zustand erneut. Die Importanforderung MK-005 ist in Foundation M2b übernommen;
ein vollständiger Jellyfin-Import ist noch nicht vorhanden.
