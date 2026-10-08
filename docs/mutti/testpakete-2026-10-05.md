# Teststand vom 5. Oktober 2026

Dies ist ein **lokaler Foundation-Teststand**, kein fertiges Gesamtprodukt und
keine Freigabe für Veröffentlichung oder produktive Migrationen. Die bestehenden
Repositories und der bestehende kurtz-Client wurden weiterentwickelt.

## Pakete und schneller Einstieg

| Paket | Absoluter Pfad | Geprüfter Umfang |
| --- | --- | --- |
| Mutti Mac | `/Users/ai/workspace/mutti/build/testpakete/mutti-foundation-2026-10-05-macOS-arm64` | Apple Silicon; eigene Mac-App und gemeinsamer Server/Web-Kern |
| Mutti Docker | `/Users/ai/workspace/mutti/build/testpakete/mutti-foundation-2026-10-05-docker-arm64` | Linux arm64 unter Docker Desktop auf diesem Mac; noch keine physische NAS-Abnahme |
| kurtz Mac | `/Users/ai/workspace/vela-swiftfin/build/testpakete/kurtz-mutti-0.9.7-78-macOS` | Universeller Catalyst-Build, macOS 15.6+, vorhandene Apple-Entwicklungssignatur |
| Gesammelte Belege | `/Users/ai/workspace/mutti/build/testpakete/mutti-pruefnachweise-2026-10-05` | Bereinigte Tests, Herkunft/Hashes, Modell- und Dienstqualifikation |

**Empfohlener erster Test: isolierter Mac-Server und kurtz auf demselben Mac.**

1. Im Mac-Paket `Start-Mac-Test.command` starten. Das Terminal bleibt während
   des Tests offen. Der Starter verwendet ausschließlich neue private
   `Testdaten` neben dem Paket; die laufende persönliche Mutti-App bleibt erhalten.
2. [Einrichtung öffnen](http://127.0.0.1:31594), **Neu einrichten** wählen und
   einen neuen lokalen Testbesitzer anlegen. Als Filmordner den vollständigen
   `Testmedien`-Pfad aus dem Terminal wählen. Der Ordner enthält nur einen
   selbst erzeugten 30-Sekunden-Testfilm.
3. Nach Abschluss [Mutti öffnen](http://127.0.0.1:31596/web/#/mutti).
   Unter **Geräte** ein Wiedergabeprofil anlegen und eine Einladung erstellen.
   Vor dem Erzeugen der Einladung die kurtz-App aus dem neuen Paket öffnen;
   Einladungen laufen zeitlich begrenzt ab.
4. In kurtz **Mit Mutti koppeln**, Link einfügen, in Mutti die passende Anfrage
   dem Testprofil zuordnen. kurtz öffnet danach direkt das Profil. Film abspielen,
   vor-/zurückspringen, kurtz beenden und erneut öffnen.
5. In Mutti die Bibliotheks-/Wiedergaberechte ändern und die Wirkung prüfen.
   Unter **Speicher** eine Sicherung erstellen, danach mit dem Testbesitzer
   **Prüfen** ausführen. Nur an diesen Testdaten eine Wiederherstellung testen:
   erneut anmelden und Geräte neu koppeln; der vorherige Datenstand bleibt erhalten.
6. Das Testgerät widerrufen und in kurtz einen neuen Bibliotheksabruf auslösen.
   Der weitere Zugriff wird serverseitig verweigert. Bereits angezeigte oder
   gepufferte Inhalte können nicht rückwirkend zurückgeholt werden.
7. Den isolierten Server mit **Ctrl-C** im Starter-Terminal beenden. Testdaten
   bleiben erhalten. Der Starter lässt sich erneut ausführen.

Dieser Starter stellt nur einen Vermittler auf Loopback bereit. Er ist bewusst
ein **Test auf demselben Mac**; für iPhone oder WAN ist dieser Einladungslink
nicht erreichbar. Keine zusätzliche Produktidentität oder Demo-Oberfläche.

Die beigelegte `Mutti.app` ist die normale native Hülle. Sie verwendet wie bisher
`~/Library/Application Support/Mutti Preview` und die Ports 18594–18596.
Vor ihrem Start die bisherige Mutti-App normal beenden. **Das ist kein isolierter
Test:** sie verwendet den vorhandenen Mutti-Datenstand. Keine Originalbibliothek
für Restore-/Widerruf-/Fehlertests einsetzen.

## Docker

Docker muss bereits laufen. Zuerst die normale Mutti-Mac-App beenden, weil der
Container die bekannten Ports 18594, 18595, 18597 und 18599 benötigt.
`Start-Docker-Test.command` lädt das mitgelieferte Image und startet ausschließlich
das Compose-Projekt `mutti-foundation-local-test`. Einrichtung unter
[localhost:18594](http://127.0.0.1:18594), Verwaltung unter
[localhost:18597](http://127.0.0.1:18597/web/#/mutti); Medienpfad im Container ist
`/media`. `Stop-Docker-Test.command` beendet dieses Projekt ohne Datenlöschung.

Verwaltungsports und Vermittler sind nur lokal veröffentlicht. Dieses Testpaket
ist keine öffentliche NAS-Konfiguration. Es enthält keine KI-, Immich- oder
Paperless-Container. Ein ARM-Image ist keine x86-NAS-Freigabe.

## Tatsächlicher Fertigstellungsstand

**Implementiert und lokal durchlaufen:** Einrichtung und Jellyfin-12.1-Übernahme,
Bibliotheken, Besitzer-/Profilgrenzen, Gerätekopplung und laufender Widerruf,
Bibliotheks-/Wiedergaberechte, eigene Verwaltung einschließlich Fehlerzuständen,
konsistente lokale Sicherung, isolierte Wiederherstellungsprobe, echter Restore,
begrenzter Wiederanlauf; native Kopplung, Mediennutzung und Wiederverbindung.
Details und Grenzen: [Foundation-Belege](evidence/foundation-2026-10-05.md).

**Qualifiziert, noch nicht als Produktmodul integriert:** drei lokale Modelle
mit 540 Antworten; echte Immich-/Paperless-APIs mit getrennten Nutzern und
Paperless-Restore in einer zweiten leeren Instanz. Keines der getesteten Modelle
ist freigegeben. Die Quellenzuordnung und Teile der Tool-/Dialogqualität erfüllen
die Abnahme noch nicht. [KI](evidence/model-casting-2026-10-05.md),
[Dienste und Entscheidung](modules-qualification.md).

**Weiterhin Implementierungsarbeit, keine offene Nutzerentscheidung:**
schlanken KI-Harness qualifizieren und integrieren; Engine-/Modellverwaltung,
Gesprächsspeicher, Werkzeuge und native Find/Ask-Strecke; profilgebundene
Immich-/Paperless-Adapter samt nativen Ansichten; verwaltete Installation,
Foto-Upload/-Sicherung, OCR-Importstrecke und belegte Antworten; kontrollierte
Updates, externes Sicherungsziel, Vollverlust-/Versionswechsel-Restore sowie
vollständige DE/EN- und Accessibility-Abnahme; außerdem vollständige Quell-/
Hinweispakete für den nativen Transport. Die Module in der Verwaltung sind
weiterhin ausdrücklich Vorschauen. Diese Arbeiten werden nicht als erledigt
oder als vom Owner blockiert dargestellt.

## Mitwirkung des Owners, nach Priorität

1. **Reale Testumgebung:** unterstützte NAS-Hardware und iPhone/iPad sowie zwei
   getrennte Internetanschlüsse bereitstellen. WAN, Netzwechsel, gesperrtes UDP,
   CGNAT/IPv6 und iOS-Hintergrundverhalten sind durch diesen Mac nicht belegbar.
2. **Öffentlicher Vermittler erst für eine Fernzugriffs-Beta:** Betreiber,
   HTTPS-/STUN-Ziel, Betriebskosten und Freigabe festlegen. Es wurde nichts
   gebucht, veröffentlicht oder nach außen freigeschaltet.
3. **Distribution/Rechte:** soweit weiterhin benötigt, externe Rechtebestätigung
   für die ungeklärte StatefulMacro-Nutzung; andernfalls bleibt ihr Ersatz
   Entwicklungsarbeit. Später passende Developer-ID-/Notarisierungs- und
   Store-Freigaben. Die vorhandene lokale Signatur ist keine öffentliche
   Distributionsfreigabe; bestehende Release-Sperren bleiben wirksam.
4. **Produkt-/Feinschliffreview:** Einrichtung, Rollenbegriffe, Kopplung und
   Wiederherstellung im Testpaket beurteilen. Der reversible Standard beim
   Restore lautet: alte Geräte und Sitzungen verlieren Zugriff, anschließend
   neu koppeln. Anschließend Navigation, Sprache, Dichte und Fehlermeldungen
   der bestehenden Produktoberflächen beurteilen.

Haussteuerung bleibt später. Keine realen Geräte wurden gesteuert. Keine
kostenpflichtige Buchung, kein Store-/TestFlight-Upload und kein Deployment.
