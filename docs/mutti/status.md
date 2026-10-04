# Umsetzungsstand

Stand: 4. Oktober 2026. **Lokale Entwicklungsvorschau; der Gesamtplan ist nicht abgeschlossen.**

Die Forks `ralleur/mutti` und `ralleur/mutti-web` behalten ihre vollständige
Jellyfin-Historie. Die Produktbranches beginnen beim zusammenpassenden stabilen
Stand v12.1. Die Umsetzung liegt zunächst auf `codex/mutti-foundation`; `main`
ist die Review-Basis. Der Web-Commit ist im Komponentenmanifest festgelegt.

| Etappe | Ergebnis | Noch offen |
| --- | --- | --- |
| M0 | Unveränderte Mac-Server- und Web-Referenz gebaut. Zwei echte tsnet-Knoten übertragen lokal verschlüsselt Daten mit ausschließlich STUN; kein DERP-Server läuft. | Getrennte Internetanschlüsse, gesperrtes UDP, CGNAT/IPv6, Netzwechsel; verbindliche Transport-/Control-Entscheidung. |
| M1 | Eigene Forks, Komponenten-Pins, Mac- und Docker-Builds, Entwicklungsanleitung, Sicherheitsregeln und CI implementiert. Lokale API-/Videodatenstrecke auf beiden Paketen bestanden. | Vollständiges Quell-/Lizenzinventar für Distribution, weitere Architekturen und vollständige Wiedergabeabnahme. |
| M2 | Eigene Mutti-Vektormarke, Sora, kurtz-Farben, Web-Assistent; native Mac-Hülle mit Serverstart, Status, Ordnerdialog-Brücke und getrennten Daten; nicht privilegiertes Docker-Paket mit schreibgeschützten Medien. | Native Ordnerauswahl durch alle Dialogschritte, Screenreader/Hellmodus vollständig, echte NAS-Installation; einfache sichere Verwaltung von einem zweiten Gerät. Der derzeitige NAS-SSH-Tunnel ist nur ein Entwicklerweg. |
| M3 | Bestehende Jellyfin-Anmeldung und Quick Connect mit normalem Wiedergabeprofil getestet; Geräteentfernung sperrt nachfolgende API-Aufrufe. Keine Codes/Secrets mehr in den bearbeiteten Quick-Connect-Logs. | Kryptografische QR-Kopplung, gerätegebundene Schlüssel, Einmaligkeit/Ablauf, Wiederherstellung, laufende Streams beenden, kurtz-Integration. Quick Connect erfüllt dieses Gate ausdrücklich nicht. |
| M4 | Kein produktiver Fernzugriff eingebaut. Kein Relay im Paket. | Accountloser Vermittlungsdienst, direkte sichere Verbindung, alle Client-Datenwege und Netzwechsel. |
| M5 | Lokale Sicherheits- und Integrationstests vorhanden. | Backup/Restore, Upgrade, Langzeittests, reale iOS-/tvOS-Geräte, NAS und zwei echte Anschlüsse. |
| M6 | Lokale Mac-App und Docker-Image verfügbar. | Gemeinsame vollständige Abnahme, Developer-ID/Notarisierung, Quellpakete und freigegebenes Release. |

## Tatsächlich geprüft

- .NET 10.0.401, Node 26.7.0, Xcode 27.0, Go 1.27.1; Linux arm64 im lokalen Docker.
- Unveränderte Jellyfin-v12.1-Referenz für den Mac-Server und Web gebaut.
- Mutti-Server: zwölf Tests gegen fremde Hosts/Origins, gefälschte Forwarded-Header,
  nicht lokale Zugriffe und ungültige Preview-Konfiguration bestanden.
- Mutti Web: Produktionsbuild und TypeScript; ESLint für geänderte Abläufe,
  Stylelint für geänderte Styles. Web-CI des ersten gepinnten Stands erfolgreich.
- Regionale Sprachauswahl wurde nach einem echten WKWebView-Sichttest korrigiert
  und gegen `de-DE`, `de_AT`, `en-gb` und ungültige Sprachcodes getestet.
- Mac und Docker: jeweils frische Daten, Besitzerzugang, nicht administratives
  Wiedergabeprofil, synthetisches 12-Sekunden-Video importiert, Quick Connect erst
  nach Besitzerfreigabe, Admin-Endpunkt verweigert, HTTP-Range `206` bytegenau
  geprüft, nach Geräteentfernung weitere authentifizierte Anfragen verweigert.
- Mac-App tatsächlich gestartet, Assistent sichtbar; App-Beenden entfernt den
  lokalen Server-Listener. Lokale Ad-hoc-Signatur verifiziert. Keine Notarisierung.
- `directlab`: echter Userspace-WireGuard-Datenweg und Disco-Endpunkt, weder DERP
  noch Peer-Relay. Ein Rechner; keine Aussage über NAT-Erfolgsquoten im Internet.

Die Testberichte unter `docs/mutti/evidence/` enthalten keine Testpasswörter,
Zugriffstokens, realen Medien oder Benutzerkonten. Vollständige Build-/Serverlogs
bleiben lokal unter `build/` bzw. im temporären Testverzeichnis.

## Nächste notwendige Entscheidung

Der direkte Aufbau muss zwischen zwei getrennten Anschlüssen geprüft werden,
bevor der Transport im Produkt festgelegt und die QR-Vertrauensbindung darauf
aufgebaut wird. Dafür wurden NAS-Plattform und ein erreichbarer Testrechner
außerhalb des Heimnetzes beim Owner angefragt; diese Angaben stehen noch aus.
Danach folgen Trennung der Haushalte, feste Mediendienst-Freigabe und eine
kurzlebige, bestätigungspflichtige Gerätekopplung. Der Test-Control-Server wird
unter keinen Umständen zu einem Produktdienst umbenannt.

Day 2 bleibt separat: Relay-Marktprüfung erst dann neu durchführen. Es wurde
kein Hosting gebucht, kein Cloudflare-Dienst angelegt und kein Relay aktiviert.
