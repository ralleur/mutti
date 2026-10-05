# Mutti – Produkt- und Umsetzungsplan

Stand: **5. Oktober 2026**. Aktuelle QR-/Transport-Umsetzung: [connect.md](connect.md). Status: **Umsetzung beauftragt und begonnen; noch kein Release**.

Mutti wird das Server-Gegenstück zu **kurtz**: ein eigenständiges, kuratiertes
Produkt auf Jellyfin-Basis. Das wichtigste Ziel ist eine einfache Einrichtung
und Kopplung: Medien auswählen, Gerät bestätigen, schauen. Nutzer benötigen
dafür kein externes Benutzerkonto und keine Kenntnisse über VPNs, Ports oder DNS.

Dieser Plan umfasst die Forks, eigene Repositories, das Designsystem, die Marke,
Installation, Gerätefreigabe, direkten Fernzugriff, Updates und Release-Prüfung.
Er übernimmt die technischen Erkenntnisse der
[Connect-Prüfung](https://github.com/ralleur/kurtz/blob/kurtz/docs/kurtz-connect-feasibility.md), ersetzt aber deren frühere
Produktempfehlung für eine verpflichtende externe Tailscale-Einrichtung.

## 1. Verbindliche Entscheidungen und Planannahmen

| Thema | Festlegung |
| --- | --- |
| Produktname | **Mutti**. `mutti` ist der technische Name für Repository, Pakete und Dateipfade. Eine spätere Wortmarke ändert diese Namensentscheidung nicht. |
| Grundlage | Eigener Jellyfin-Fork mit erhaltener Historie und möglichst kleinen, klar abgegrenzten Anpassungen. |
| Client | kurtz bleibt ein eigenständiges Produkt und weiterhin mit gewöhnlichen Jellyfin-Servern kompatibel. |
| Gestaltung | Vorhandenes kurtz-Designsystem als Grundlage; eigene Mutti-Identität und passende Server-Oberfläche. |
| Erste Auslieferung | **Mac-App und Docker/NAS gemeinsam**, mit identischem Serverstand und gemeinsamem Funktionsumfang. |
| Konto | Kein verpflichtendes Konto bei Ralleur, Tailscale, Cloudflare oder einem anderen Anbieter für Endnutzer. Lokale Besitzeridentität und Jellyfin-Benutzerrechte bleiben notwendig. |
| Day 1 | Sichere lokale Verbindung und direkter verschlüsselter Fernzugriff, soweit beide Netze das ermöglichen. |
| Relay | **Day 2. In Day 1 kein Medien-Relay, kein stiller Rückfall auf öffentliche DERP-, TURN- oder Peer-Relays.** |
| Spätere Anbieterauswahl | Erst zum Start von Day 2 den Markt erneut prüfen, insbesondere kostenlose Angebote und deren reale Grenzen. Heute keine Anbieterbindung und kein dauerhaftes Gratisversprechen. |
| Netzwerkfehler | Verständliche Meldung und erneuter Versuch; niemals unverschlüsselter oder öffentlich freigeschalteter Ersatzweg. |
| Aktueller Auftrag | Plan ausführen. Eigene Forks, lokale Implementierung und überprüfbare Builds sind beauftragt. Hosting und öffentliche Release-Freigabe folgen nach den jeweiligen Abnahmen. |

**Plattformentscheidung:** Mac-App und Docker/NAS gehören gemeinsam zu Day 1.
Beide Auslieferungen bauen auf demselben plattformübergreifenden Kern und
derselben Weboberfläche auf. Installation und Betrieb werden pro Plattform
geprüft; eine fertige Mac-App allein erfüllt das erste Release noch nicht.
Windows-Installer und herstellerspezifische NAS-Pakete ohne Containerbetrieb
gehören zunächst zum späteren Ausbau.

**Architekturvorschlag:** Ein kleiner erreichbarer Dienst vermittelt Kopplung
und Verbindungsinformationen. STUN hilft beim Ermitteln erreichbarer Adressen.
Beide übertragen keine Bibliotheken oder Videodaten. „Ohne Relay“ bedeutet somit
keine Garantie für einen vollständig dienstlosen Fernzugriff. Betreiber,
Hosting, Betriebskosten und Ausfallverhalten dieses Vermittlungsdienstes werden
vor einer öffentlichen Fernzugriffs-Beta festgelegt; ein Hostingauftrag entsteht
durch diesen Plan nicht.

## 2. Was die erste Version leisten soll

Die Erstinstallation bietet gemäß MK-005 zuerst **Neu einrichten** oder
**Aus Jellyfin übernehmen** an (M2b, für 12.1 als Teststand umgesetzt). Danach folgen die jeweilige
Einrichtung oder Importprüfung und erst nach deren Erfolg die erste Gerätefreigabe. Fortschritt und Fehler bleiben verständlich, auch
während Jellyfin die Bibliothek noch einliest. Erweiterte Einstellungen sind
erreichbar, stehen aber nicht im ersten Einrichtungsablauf.

Zum Pflichtumfang gehören:

- Filme und Serien über die bestehende Jellyfin-Bibliothek und Wiedergabe.
- Einrichten, Ändern und Prüfen von Medienordnern; fehlende oder abgezogene
  Datenträger erkennen, ohne Medien oder Bibliotheksdaten ungefragt zu löschen.
- QR-Kopplung, Geräteübersicht, Rechtevergabe, Sperren und erneute Freigabe.
- Lokaler Besitzerzugang mit Wiederherstellungsmöglichkeit; ein Wiedergabeprofil
  erhält keine Administratorrechte.
- Verbindung im Heimnetz, direkter Fernzugriff, Wiederverbinden nach Neustart
  und Netzwechsel, nachvollziehbare Fehlermeldungen.
- Status für Server, Bibliothekslauf und Fernzugriff; freiwilliger, bereinigter
  Diagnoseexport.
- Kontrollierte Updates, Sicherung und tatsächlich geprüfte Wiederherstellung.
- Deutsch und Englisch, Tastaturbedienung, Screenreader, Hell-/Dunkelmodus.

Nicht Bestandteil von Day 1 sind Relay, universelle Erreichbarkeitsversprechen,
ein allgemeines VPN, Zugriff auf das gesamte Heimnetz, Exit-Node-Funktionen,
eine neue Transcoding-Engine oder ein vollständiger Neubau sämtlicher Jellyfin-
Verwaltungsfunktionen. Die zuvor ausgeschlossene Jellyfin-Übernahme ist durch die
Owner-Rückmeldung vom 5. Oktober als M2b in den Umfang aufgenommen worden.
Jellyfin-Wiedergabe im Browser bleibt erhalten; kurtz ist der bevorzugte Client.

## 3. Forks und eigene Repositories

Jellyfin trennt Server und Webclient. Deshalb ist für ein vollständig angepasstes
Produkt neben dem Server-Fork auch eine gepflegte Web-Basis notwendig.
Quellen: [Jellyfin Server](https://github.com/jellyfin/jellyfin),
[Jellyfin Web](https://github.com/jellyfin/jellyfin-web).

| Vorgesehener Name | Aufgabe |
| --- | --- |
| `ralleur/mutti` | Hauptrepository und Fork von `jellyfin/jellyfin`: Server, Produktdokumentation, Verbindungsdienst, Mac-Hülle, Paketierung, Releases und zentrale Aufgabenliste. |
| `ralleur/mutti-web` | Technisches Begleitrepository und Fork von `jellyfin/jellyfin-web`: Mutti-Einrichtung, Geräteverwaltung, angepasste Weboberfläche und Web-Komponenten. |
| Bestehendes `ralleur/kurtz` | Kopplung, Client-Transport, Verbindungszustände und Wiedergabeintegration auf den Apple-Plattformen. |

Die Namen wurden im Umsetzungsauftrag geprüft und beide öffentlichen Forks
unter `ralleur` angelegt. Der konkrete Stand und die noch offenen Release-Gates
stehen in [status.md](status.md).

Vorgehen für die Repository-Einrichtung:

1. Server- und Web-Fork mit vollständiger Upstream-Historie erstellen. Den
   Upstream-Remote beibehalten; keine Kopie ohne Herkunft und keine pauschale
   Umbenennung von Jellyfin-/Emby-Namensräumen.
2. Einen zusammenpassenden, unterstützten stabilen Server-/Web-Stand auswählen.
   Tags und Commit-Hashes festhalten. Die im alten PoC verwendete Version ist
   keine automatische Release-Empfehlung.
3. Vor Änderungen einen unveränderten Build und eine frische Testinstallation
   reproduzieren. SDK, Node, .NET, FFmpeg und weitere Werkzeuge versionieren.
4. Mutti-Anpassungen als kleine Feature-Commits pflegen; Aufgabenbranches erhalten
   den Präfix `codex/`. Die Produktlinie beginnt beim gewählten stabilen Stand.
5. README, AGENTS, CONTRIBUTING, SECURITY, Lizenzhinweise, Supportwege,
   Issue-/PR-Vorlagen und Release-Regeln anlegen. Bestehende Mitwirkende nennen.
6. Hauptbranch schützen und passende CI-Prüfungen verlangen. Geheimnisse,
   Signaturschlüssel und reale Medien bleiben außerhalb des Repositories.

Vorgesehene Ergänzungen im Hauptrepository, ohne Umbau des Upstream-Baums:

```text
mutti/
  apps/macos/          native Installation und Dienststeuerung
  connect/             lokaler Verbindungsdienst und Zugriffsprüfung
  control/             Kopplung und Vermittlung ohne Medienweiterleitung
  contracts/           versionierte Nachrichten und Kompatibilitätsfälle
  design/              semantische Tokens, Schrift- und Markenquellen
  packaging/           Mac- und Docker-Pakete desselben Releases
  tests/               Produkt-, Verbindungs- und Update-Prüfungen
  components.lock.json zusammenpassende Versionen und Quell-Hashes
docs/mutti/            Entscheidungen, Betrieb und Release-Nachweise
```

Der Web-Fork wird für jeden Build auf einen konkreten Commit festgelegt.
Generierte Webdateien und Frameworks werden nicht manuell in den Server-Fork
kopiert. Das Release-Manifest verbindet Mutti-, Jellyfin-, Web-, Transport- und
FFmpeg-Versionen; Mutti erhält eine eigene Versionsnummer.

## 4. Architektur und entscheidender Vorversuch

Die vorgeschlagene Struktur trennt Medienverarbeitung, Kopplung und Oberfläche:

```text
kurtz ── direkte authentifizierte, verschlüsselte Verbindung ── Mutti Connect
                                                                │
                                                   lokaler Jellyfin-Server

kurtz und Mutti Connect ── Kopplung/Adressvermittlung ── Control-Dienst
kurtz und Mutti Connect ── Adressermittlung ── STUN
```

Der Jellyfin-Prozess übernimmt Medienbibliothek, Benutzerrechte und Transcoding.
Der Connect-Dienst stellt nur den freigegebenen Mediendienst bereit, ohne
Heimnetz-Routing oder beliebige Proxy-Ziele. Die native Mac-Hülle übernimmt
Systemintegration; die gemeinsame Weboberfläche dient vom ersten Release an
der Verwaltung vom Handy und der Docker-/NAS-Installationen.

**Erstes technisches Gate:** Der vorhandene `tsnet`-PoC hat direkte Wiedergabe
gezeigt, aber kein Produktionssystem ohne Relay nachgewiesen. DERP wird im
Tailscale-Stack auch bei der Verbindungsanbahnung eingesetzt. Eine als „direkt“
angezeigte laufende Verbindung beweist nicht, dass vorher oder beim Netzwechsel
keine Daten über einen Relay liefen.

Vor einer verbindlichen Transportentscheidung wird deshalb isoliert geprüft:

- Lassen sich sämtliche Medien-Relays abschalten, einschließlich impliziter
  Standardserver, Peer-Relays und automatischem Rückfall nach Verbindungsabbruch?
- Funktionieren Adressvermittlung, STUN und direkter Aufbau dann noch mit
  vertretbaren, wartbaren Schnittstellen auf Mac, iOS und tvOS?
- Können sich die Geräte beim Wechsel von WLAN zu Mobilfunk neu verbinden,
  ohne dafür einen Relay zu aktivieren?
- Gibt es einen nachweisbaren Zustand `directUnavailable`, einen begrenzten
  Aufbauversuch und eine sichere Rückkehr zur lokalen Verbindung?
- Bleibt die Integration an gepflegten Bibliotheken ausgerichtet, ohne eigenes
  kryptografisches Verfahren oder einen umfangreichen Fork von Tailscale?

`tsnet`/`libtailscale` bleiben Kandidaten, keine beschlossene Abhängigkeit. Wenn
der Vorversuch scheitert, folgt ein dokumentierter Vergleich geeigneter
vorhandener Bausteine für direkten Verbindungsaufbau. Relay wird dabei nicht
heimlich wieder aufgenommen. Ein ausschließlich lokaler Zwischenstand kann als
solcher getestet werden; er erfüllt noch nicht den geplanten Day-1-Fernzugriff.

Control muss bestehende Protokolle und Komponenten verwenden, wo sie geeignet
sind. Headscale ist ein Kandidat für die Steuerung, aber keine bereits
qualifizierte Lösung für einen Dienst mit vielen voneinander getrennten
Haushalten. Weder der Test-Control-Server aus dem PoC noch eine unbeschränkte
gemeinsame Gerätewolke werden als Produktionslösung übernommen.

## 5. Kopplung vom ersten Start bis zum Film

**Erste Einrichtung am Server:** Mutti erzeugt lokal die Serveridentität und
bindet den Besitzer in einem lokalen, geschützten Ablauf. Auf dem Mac kann dies
aus der installierten App heraus beginnen. Für Docker wird ein einmaliges
Einrichtungsgeheimnis benötigt; „der erste Besucher im LAN wird Administrator“
ist kein zulässiger Ersatz. Erst nach abgeschlossener Besitzerzuordnung wird
Fernzugriff aktivierbar. Der Besitzer kann ein Handy als Verwaltungsgerät
hinzufügen und erhält einen Wiederherstellungsweg.

**Weiteres Wiedergabegerät:**

1. kurtz erzeugt einen Geräteschlüssel und zeigt eine zeitlich begrenzte
   Kopplungsanfrage als QR-Code. Für Mac/iPhone gibt es zusätzlich eine passende
   lokale Geräteauswahl beziehungsweise Code-Eingabe, sodass kein Gerät seinen
   eigenen Bildschirm scannen muss.
2. Der Besitzer öffnet die Anfrage auf seinem bereits berechtigten Handy oder
   in Mutti. Er wählt den Server und das Wiedergabeprofil und bestätigt das neue
   Gerät. Lokale Kopplung soll ohne öffentlichen Vermittlungsdienst funktionieren.
3. Die Bestätigung bindet Geräteidentität, Serveridentität, Profil und konkrete
   Anfrage zusammen. Nur dann werden kurzlebige Bootstrap-Daten ausgegeben.
4. kurtz prüft die bekannte Serveridentität, stellt den geschützten direkten
   Kanal her und erhält eine auf dieses Gerät und Profil begrenzte Sitzung.
5. Die Bibliothek erscheint; bei laufendem Erstscan bleibt dessen Status sichtbar.

Jellyfin Quick Connect kann die Profilanmeldung unterstützen. Seine Codes lösen
keine Netzwerkerreichbarkeit und ersetzen nicht die Besitzerfreigabe für
Fernzugriff. [Upstream-Verhalten](https://jellyfin.org/docs/general/server/quick-connect/).

**Erstkopplung aus der Ferne:** Die Anfrage benötigt den erreichbaren
Vermittlungsdienst sowie einen Besitzer, der bereits Zugriff auf Mutti hat.
Bestätigtes Pairing garantiert noch keine direkte Netzwerkverbindung. Wenn kein
direkter Weg entsteht, bleibt die Berechtigung erhalten und die App meldet den
Verbindungsfehler. Eine Freigabe ist kein Anlass, Passwörter oder Zugriffsrechte
zu lockern.

Geplante Nutzerzustände: „Gerät bestätigen“, „Warte auf Freigabe“, „Verbinden“,
„Verbunden“, „Server nicht erreichbar“, „Direkte Verbindung derzeit nicht
möglich“, „Einladung abgelaufen“ und „Zugriff entfernt“. Bei einem Fehlschlag
zeigen wir einen erneuten Versuch und verständliche Hinweise zum Netzwechsel.
Ein getrennt erreichbarer Diagnosebereich darf technische Details enthalten.
Wir behaupten bei einem unklaren Timeout keine sicher erkannte Routerursache.

### 5.1 — Reihenfolge nach dem ersten Nutzertest

**Owner-Rückmeldung 05.10.2026:** Während der Ersteinrichtung keine Kopplung und
keinen Fernzugriff anbieten. Ein erreichbarer Server ist noch kein eingerichteter
Server. Maßgeblich ist `StartupWizardCompleted` aus `System/Info/Public`, nicht
`/health`. Fehlende/ungültige Zustandsdaten geben die Kopplung nicht frei.
Die Mac-Hülle startet den Kopplungsdienst erst nach der bestätigten Fertigstellung.

### 5.2 — M2b: Bestehendes Jellyfin übernehmen (MK-005)

**MK-010, beauftragt am 05.10.2026:** Kurt aus Hauser begleitet jeden der sieben
Importschritte mit der vom Owner festgelegten Animation. Die echte Statusanzeige
bleibt sichtbar, die Figur ist pausierbar und berücksichtigt reduzierte Bewegung.
Lokale Ressourcen, identischer Mac-/Docker-Pfad; Zustände und Auslieferung werden
mit künstlichen Importdaten geprüft. [Umsetzung](import.md#kurt-begleitet-den-umzug-mk-010).

**Ergänzung 05.10.2026:** Die automatische Importvorbereitung übernimmt bei
Jellyfin-SQLite den Wechsel vom problematischen Sperrmodus auf den Standard,
sichert die Originalkonfiguration und begleitet den Neustart. Der Hinweis beim
Importstart genügt; Nutzer suchen keine Datenbankeinstellung. Für einen bereits
blockierten lokalen Mac-Benutzerdienst steht ein an Prozess, LaunchAgent,
Dateieigentümer und Serveridentität gebundener Wiederanlauf zur Verfügung.
API-Weg und Mac-Wiederanlauf werden getrennt mit synthetischen Quellen geprüft;
Details und Plattformgrenzen: [automatische Vorbereitung](import.md#automatische-vorbereitung-des-quellservers).

**Teststand umgesetzt für Jellyfin 12.1; Owner-Abnahme ausstehend.** Aktueller
Funktionsumfang und Grenzen: [Import-Testanleitung](import.md). Kanonischer
Ideeneintrag: [MK-005 im gemeinsamen Ideen-Eingang](/Users/ai/workspace/vela-swiftfin/IDEEN.md).

Zielablauf:

1. **Neu einrichten / Aus Jellyfin übernehmen** vor dem bisherigen Assistenten.
2. Lokale Jellyfin-Instanzen anhand erreichbarer Server und vorhandener
   Installationen erkennen; Name, Version und Adresse anzeigen. Kein automatischer
   Zugriff auf deren Datenbank. Die laufende Mutti-Instanz ausschließen.
3. Alternativ Adresse und Administratorname/Passwort eingeben. Anmeldung nur an
   der ausdrücklich ausgewählten Quelle, keine Weiterleitung von Zugangsdaten
   an Redirect-Ziele. Zugangsdaten nur für die Übernahme halten, nicht protokollieren.
4. Version, Besitzerrechte, Datenumfang, Medienpfade und verfügbare Zielressourcen
   prüfen. Vor Beginn eine konkrete Übernahmeübersicht zeigen.
5. Konsistenten Stand in einen getrennten Mutti-Datenbereich übernehmen,
   Ergebnis prüfen und erst danach aktivieren. Die Quelle bleibt bis zum
   kontrollierten Wechsel erhalten; fehlgeschlagene/unterbrochene Importe
   müssen ohne Halbzustand erneut ausführbar sein.
6. Zusammenfassung anzeigen, mit bestehendem Benutzer anmelden und anschließend
   Geräte koppeln. Keine erneute manuelle Einrichtung bereits übernommener Inhalte.

Pflichtumfang sind Bibliotheksdefinitionen samt Medienzuordnung und Metadaten,
Benutzer und Rechte, bestehende lokale Benutzeranmeldung, Wiedergabestand
(einschließlich Resume-Position, Zähler und letztem Wiedergabedatum), Favoriten,
Playlists/Sammlungen und kompatible Server-/Benutzereinstellungen. Vor Freigabe
an synthetischen Daten je Benutzer vergleichen; ein bloßer Neuscan zählt nicht
als erfolgreiche Übernahme. Medien verbleiben an ihren Speicherorten, sofern
nicht ausdrücklich ein Dateiumzug gewählt wurde; der Zielserver muss sie lesen
können. Fehlende Laufwerke/Mounts sind vor Aktivierung aufzulösen.

**Verifizierte technische Grenze:** Im gepinnten Jellyfin-Stand bietet
`BackupController` Erstellen, Auflisten, Manifestlesen und Wiederherstellen,
aber keinen Archivdownload. Die normale Admin-API exportiert auch keine lokalen
Passworthashes. Adresse und Adminanmeldung allein erlauben daher keinen
vollständigen Fernimport. Datenbank-/Konfigurationssicherung oder ein zusätzlicher
Exportweg sind erforderlich. Der ursprüngliche Vollumfang bleibt bestehen;
ein API-Teilimport darf nicht als vollständiger Umzug bezeichnet werden.

**Präzisierung des Owners, 05.10.2026: möglichst Ein-Klick-Import.** Die
Sicherung ist ein interner Arbeitsschritt. Eine manuell erstellte/ausgewählte ZIP
ist nicht der Standardablauf. Die vorher offene Produktwahl „Archiv oder Helfer“
ist damit durch die Priorität eines automatisierten Umzugs ersetzt.

- **Quelle auf demselben Mac:** lokales Jellyfin erkennen, ausgewählten Server
  bestätigen und erforderliche Anmeldung beziehungsweise macOS-Dateifreigabe
  einholen. Nach der Übernahmeaktion die Online-Sicherung über Jellyfin anstoßen,
  den zurückgegebenen lokalen Archivpfad prüfen und die abgeschlossene Sicherung
  direkt lesen. So fehlt kein HTTP-Download-Endpunkt. Zusatzdaten außerhalb der
  eingebauten Sicherung, insbesondere Plugin-Konfigurationen, gesondert erfassen.
  Pfade aus der Quelle gelten nur nach Prüfung als lokale Installationspfade;
  kein beliebiger Dateizugriff allein aufgrund einer Serverantwort.
- **Quelle auf einem anderen Rechner/NAS:** einen temporären Mutti-Umzugshelfer
  qualifizieren, vorzugsweise als versionsgebundenes Jellyfin-Plugin. Mutti soll
  Einrichtung, Export und verschlüsselte Übertragung steuern. Das ist erst nach
  ausdrücklicher Freigabe des Betreibers zulässig; mögliche Serverneustarts und
  Betriebsunterbrechungen stehen vor dem Start fest. Plugin-Installation über
  Admin-APIs, HTTPS-Erreichbarkeit und der tatsächliche vollständige Export sind
  technische Gates, noch keine zugesicherte Funktion aller Jellyfin-Versionen.
  Der Export ist kurzlebig, nur für den bestätigten Empfänger und den festgelegten
  Datenumfang zugänglich. Nach Abschluss Zugriff sofort deaktivieren, Paket und
  temporäre Daten über einen geprüften Bereinigungsablauf entfernen. Einen noch
  nötigen Neustart offen ausweisen; keine falsche Selbstlöschungszusage.
- **Geplanter Ausweichweg:** Manueller Archivimport für nicht unterstützte
  Installationen ist in dieser Vorschau noch offen. Docker-Volumes, Dateirechte oder nicht erreichbare Medien können
  eine zusätzliche gezielte Freigabe erfordern. Mutti erkennt diese Fälle vor
  dem Wechsel und bietet genau den nötigen nächsten Schritt an.

Vorgesehene Journey: **Jellyfin gefunden → Übernehmen → ggf. Anmeldung/Freigabe →
Prüfen und Übertragen → Fertig → Geräte koppeln.** Ein bereits erreichbarer,
kompatibler und berechtigter lokaler Server soll eine einzige Startaktion
brauchen. Bei Konflikten keine stillen Standardentscheidungen, die Daten oder
Rechte verlieren. Der alte Server wird nicht als Teil der Erkennung beendet;
Fortschritt und Rückkehrmöglichkeit bleiben sichtbar. Änderungen auf der Quelle
nach dem Snapshot benötigen eine definierte finale Synchronisierung oder ein
abgestimmtes Wechselzeitfenster, damit neuer Wiedergabestand nicht verloren geht.

Eine laufende SQLite-Datenbank wird nicht unkoordiniert kopiert. Jellyfins
Sicherungsfunktion kann einen konsistenten Online-Stand erzeugen; bei aktivem
Bibliotheksscan verweigert der gepinnte Stand die Erstellung. Mutti muss warten
oder den Konflikt anzeigen. Der lokale automatisierte Import sowie der
Exporthelfer wurden auf separaten synthetischen Mac-/Linux-Testinstanzen geprüft;
die breite Versions-/Plugin-/NAS-Abnahme bleibt offen. Relevante Primärquellen:
[Jellyfin Backup/Restore](https://jellyfin.org/docs/general/administration/backup-and-restore/),
[Jellyfin Migration](https://jellyfin.org/docs/general/administration/migrate/),
[`BackupController`](../../Jellyfin.Api/Controllers/BackupController.cs),
[`BackupService`](../../Jellyfin.Server.Implementations/FullSystemBackup/BackupService.cs),
[Jellyfin-Plugins](https://jellyfin.org/docs/general/server/plugins/index.html).

Versionsmigration, Pfadwechsel zwischen Mac/NAS/Docker, externe Anmeldeanbieter,
Plugins, Hardware-Transcoding und Netzwerkeinstellungen müssen gesondert geprüft
werden. Alte öffentliche Listener oder Datenbankpfade dürfen Muttis lokale
Sicherheitsgrenzen nicht überschreiben. Inkompatible Einstellungen werden vor
Übernahme sichtbar gemacht und erfordern eine aufgelöste Entscheidung; kein
stilles Weglassen mit einer „alles übernommen“-Meldung. Mac und Docker/NAS
gehören auch für diesen Import zur gemeinsamen Abnahme.

### 5.3 — Intro Skipper als kuratierter Standard (MK-006)

**Owner-Entscheidung 05.10.2026:** Intro Skipper gehört zu jeder Mutti-Einrichtung
und wird bei einem Jellyfin-Umzug ohne gesonderte Plugin-Rückfrage mitgenommen.
Kanonischer Eintrag: [MK-006](/Users/ai/workspace/vela-swiftfin/IDEEN.md).

- Offizielles Plugin 12.0.4.0, Quellcommit und SHA-256 im Komponentenmanifest.
  Identische DLL in Mac und Docker; kein Laufzeitdownload und keine Neuanmeldung.
- Beim Import von Intro Skipper 12.0.4.0: Konfiguration, Ausschlüsse, Segmentdaten,
  manuelle Änderungen und Analysecache über konsistente SQLite-Online-Snapshots
  übernehmen. Jellyfins eigene MediaSegments bleiben Teil des Vollimports.
- Lokale und durch den Exporthelfer übertragene Daten gleich prüfen; keine
  Übernahme fremder Pluginprogramme. Vor Aktivierung Vergleich der logischen
  Datenbankinhalte und erneute Quellenprüfung. Originaldaten bleiben bestehen.
- Ohne Plugin auf der Quelle und bei neuer Einrichtung: geprüfte Standardversion
  automatisch aktiv. Bundled FFmpeg muss die benötigten Analysefunktionen erfüllen.
- Gemeinsame Abnahme: Mac/Docker, Quelle mit und ohne Plugin, vorhandene Segmente
  und Konfiguration, FFmpeg-Funktionsprüfung, WAL-Sicherung und Neustart.
  Weitere Quell-Pluginversionen benötigen eine qualifizierte Schemaübernahme.

## 6. Sicherheits- und Datenschutzumfang

Diese Anforderungen sind Teil der Umsetzung und ihrer Abnahme:

- Schlüssel entstehen auf dem jeweiligen Gerät. QR-Codes enthalten keine
  dauerhaften privaten Schlüssel, Administratorpasswörter oder Jellyfin-Tokens.
- Kopplungseinladungen sind zufällig, kurz gültig, atomar nur einmal verwendbar
  und gegen wiederholtes Raten begrenzt. Eine manuell eintippbare Kurzform
  benötigt ein geeignetes Standardverfahren und gesonderte Missbrauchsgrenzen.
- Die Besitzerfreigabe bestätigt den konkreten Geräteschlüssel. Auch ein
  kompromittierter Vermittler darf keinen anderen Server oder Client unbemerkt
  unterschieben. Wiederherstellung und Schlüsselwechsel dürfen diesen Schutz
  nicht umgehen.
- Der Connect-Dienst erzwingt Haushalts- und Gerätegrenzen auf Serverseite.
  Netzwerkerreichbarkeit allein gewährt noch keinen Jellyfin-Zugriff.
- Administrator- und Wiedergaberechte bleiben getrennt. Es gibt keine mit der
  App ausgelieferten gemeinsamen API-Schlüssel oder universellen Cloud-Tokens.
- „Gerät entfernen“ sperrt neue Zugriffe und beendet bestehende Streams und
  WebSockets. Sperrungen bleiben nach Neustart bestehen.
- Lokale Verbindungen prüfen dieselbe bestätigte Identität. Eine Jellyfin-
  Server-ID oder WLAN-Zugehörigkeit ist kein Ersatz für kryptografische Prüfung.
  Zugriffstokens werden erst nach dieser Prüfung gesendet.
- Mac/iOS/tvOS verwenden geeignete geschützte Schlüsselspeicher; auf dem Server
  werden Geheimnisse und Zustand durch Dienstrechte und Dateiberechtigungen
  geschützt. Sicherungen mit Geheimnissen benötigen gesonderten Schutz.
- Kein allgemeiner HTTP-/SOCKS-Proxy, keine weitergereichten Fremd-URLs und keine
  ungeprüften Redirects. Reverse-Proxy-Header und Jellyfins lokale/entfernte
  Zugriffsregeln werden gezielt geprüft.
- Setup und Verwaltung erhalten Authentifizierung, Schutz vor CSRF und
  DNS-Rebinding sowie begrenzte lokale Freigaben. Jellyfin bleibt intern;
  vorhandene Legacy-LAN-Zugänge werden nur bewusst und dokumentiert angeboten.
- Keine Werbe- oder Nutzungsanalyse als Voreinstellung. Verbindungsmetadaten,
  optionale Diagnostik, Löschfristen und externe Metadatenanbieter werden erklärt.
  Logs und Supportexporte entfernen Tokens, Einladungen und private Schlüssel.

Jellyfin benötigt für Poster und Beschreibungen gegebenenfalls externe
Metadatenanbieter. „Private Medien“ bedeutet nicht, dass jede Metadatenabfrage
offline geschieht. Der Assistent erklärt diese Wahl und bietet einen Modus mit
ausschließlich lokalen Metadaten an.

## 7. kurtz-Designsystem und Mutti-Branding

Die vorgefundenen verbindlichen Grundlagen stehen im
[kurtz-Markenhandbuch](../marketing/brand/README.md), in
[KurtzBrand.swift](../Shared/Kurtz/KurtzBrand.swift) und im
[Webstylesheet](../website/style.css). Sie sind derzeit noch kein vollständiges,
plattformübergreifendes Komponentenpaket.

| Bestehende Grundlage | Übernahme für Mutti |
| --- | --- |
| Graphite `#1F1F1F` | Dunkle Flächen und Text auf hellen Flächen |
| Ivory `#FAF8F1` | Helle Flächen und Text auf dunklen Flächen |
| Electric Yellow `#FFE600` | Gemeinsamer Akzent und deutliche Hauptaktionen |
| Sora Regular, SemiBold, Bold | Lokal mitgelieferte Typografie mit Lizenzhinweisen |
| Systemschriften/-symbole | Plattformkontrollen, Symbole und diagnostische Monospace-Texte |

Arbeitspakete:

1. Aus den vorhandenen Werten eine versionierte Quelle semantischer Tokens
   erstellen: Farben, Textstufen, Abstände, Radien, Rahmen, Fokus, Status und
   reduzierte Bewegung. CSS- und Swift-Ausgaben daraus ableiten. Zusätzliche
   Werte werden bewusst für Mutti entworfen, nicht als bestehender kurtz-Standard
   ausgegeben. Keine Remote-Schriften.
2. Mutti-Wortmarke, eigenes Symbol und App-Icon entwerfen; Verbindung zur
   Produktfamilie über Farben, Typografie und Formensprache herstellen. Das
   charakteristische kurtz-Zeichen und dessen Video-Claim werden nicht schlicht
   umbeschriftet. Eine kleine Ralleur-Signatur folgt der
   [bestehenden Absenderidentität](../marketing/brand/ralleur/README.md).
3. Komponenten für Formulare, Ordnerauswahl, Status, Fortschritt, QR-Karte,
   Geräteliste, Dialoge, leere Zustände, Fehler und Wiederherstellung definieren.
   Gelb auf Ivory wird nicht als schlecht lesbare Standardschrift verwendet.
4. Die zentralen Ansichten gestalten: Willkommen, Einrichtung, Bibliotheken,
   Serverübersicht, Gerätefreigabe, Geräteverwaltung, Sicherungen/Updates,
   Fehlerhilfe und Info/Lizenzen. Auf dem Handy muss die Freigabe vollständig
   bedienbar sein; auf dem Mac bleiben native Systemdialoge erhalten.
5. Sichtbares Jellyfin-Branding in Produktflächen, Browser-Titeln, Ladebildern,
   Icons, Installern und Benachrichtigungen systematisch anpassen. Attribution,
   Lizenzen und technische Kompatibilitätsfelder erhalten. Kein pauschales
   Suchen-und-Ersetzen in APIs, Namespaces, Protokollen oder Datenbanken.
6. Neue App-/Dienst-/Paketkennungen für Mutti festlegen, zum Beispiel unter
   `com.ralleur.mutti`. kurtz-Kennungen und dessen bestehende Installationen
   bleiben erhalten. Die unabhängige Jellyfin-Herkunft wird in Info und README
   sichtbar benannt.

Abnahme anhand echter App-Ansichten und eines dokumentierten Markenpakets:
SVG-Quellen, exportierte Icons, Hell-/Dunkelvarianten, Schriftlizenzen und
reproduzierbare Exportbefehle. Tastatur, VoiceOver, Vergrößerung, Kontraste und
Statusdarstellung ohne alleinige Farbcodierung gehören zur UI-Prüfung.

## 8. Installation, Betrieb und bestehende Server

**Mac-App:** eine signierte und notarisierte App mit gebündelter
passender Laufzeit und FFmpeg. Die native Hülle führt durch Ordnerauswahl,
Berechtigungen, Start/Stop, Status und optionalen Start bei Anmeldung. Das
Schließen des Fensters und das tatsächliche Beenden des Servers sind klar
unterschieden. Ruhezustand, externe Laufwerke, Speicherplatz, Portkonflikte und
fehlende Dateirechte erhalten verständliche Behandlung. Verfügbarkeit bei
abgemeldetem Benutzer und Systemdienstinstallation werden gesondert entschieden;
ein reiner Anmeldedienst darf nicht als immer verfügbar beworben werden.

Ziel ist ein universelles Mac-Paket für Apple Silicon und Intel. Die gewählte
Upstream-Laufzeit und alle gebündelten Komponenten müssen diese Kombination
tragen. Älteste unterstützte macOS-Version und tatsächliche Architekturfreigaben
werden in M0 festgelegt und durch eigene Build-/Runtime-Prüfungen belegt.

**Docker/NAS im selben ersten Release:** versioniertes Image und verständliche Compose-Vorlage,
persistente Konfiguration und Sicherungen, Medien standardmäßig nur lesbar,
keine privilegierten Container oder pauschalen Heimnetzfreigaben. DNS, Discovery,
Dateirechte, Host-Netzwerkbesonderheiten und Hardware-Transcoding werden für
konkrete Zielsysteme qualifiziert. Ziel ist ein Multiarch-Image für `amd64` und
`arm64`; zugesagt werden nur tatsächlich geprüfte Architekturen. Mindestens
ein reales NAS mit Containerbetrieb gehört zur ersten Abnahme. Die konkrete
NAS-/Betriebssystem-Matrix wird in M0 anhand verfügbarer Hardware festgelegt.
Ein Compose-Paket ist keine pauschale Zusage für alle NAS-Hersteller oder deren
proprietäre App-Stores.

Mac- und Docker-Pakete verwenden denselben Komponenten-Lock und dieselben
Kopplungsverträge. Beide werden aus derselben Release-Pipeline erzeugt. Docker
erhält dokumentierte Versions-/Digest-Pins sowie einen Update- und Restore-Ablauf;
ein wechselndes `latest`-Image ersetzt keine geprüfte Aktualisierung. Der
Webassistent beginnt nach dem jeweiligen Installations-/Deployment-Schritt.

**Bestehendes Jellyfin:** Mutti bekommt einen eigenen Datenbereich und darf
nicht ungefragt dieselbe aktive Datenbank öffnen. Medienordner können lesend
wiederverwendet werden. Für eine spätere Übernahme werden Versionskompatibilität,
Sicherung, Wiederherstellung und Geräteidentität ausdrücklich behandelt.
Zwei Server dürfen nicht gleichzeitig dasselbe Konfigurationsverzeichnis
verwenden. Ein separater Connect-Zusatz für unverändertes Jellyfin ist eine
spätere Option, kein zusätzlicher Pflichtumfang der ersten Mutti-Version.

## 9. Integration in kurtz

Die Server-App allein kann den Ablauf nicht fertigstellen. Im bestehenden
kurtz-Repository sind folgende klar abgegrenzte Änderungen vorgesehen:

- „Mit Mutti verbinden“ in der Servereinrichtung, QR-/Code-Anfrage,
  Freigabestatus, Fehlermeldungen und Geräteidentität.
- Versionierte Kopplungsnachrichten und Fähigkeitsabfrage, damit ältere
  Clients verständliche Hinweise statt unklarer Verbindungsfehler erhalten.
- Gemeinsamer Transport für API, Bilder, WebSockets, direkte Videodaten,
  HLS-Segmente, Untertitel und die tatsächlich verwendeten Player.
- Ein nur lokal erreichbarer, abgesicherter Medienzugang mit festem Ziel, falls
  der gewählte Userspace-Transport dies verlangt; kein offener Proxy.
- Wiederaufnahme nach Schlaf, App-Neustart und Netzwechsel; Zeitlimits,
  Abbruch und Wiederholungsstrategie ohne endlose Ladeschleifen.
- Gerätebezogene Sitzung und Schlüssel im passenden geschützten Speicher;
  vollständiges Entfernen einer Serverkopplung.
- Normale Jellyfin-Anmeldung und vorhandene Servereinträge bleiben nutzbar.

macOS dient als erster Integrationsnachweis. iPhone/iPad und **echtes Apple TV**
folgen vor der jeweiligen Freigabe. Der bisherige tvOS-/Catalyst-Linktest ist
kein Nachweis für Installation, Lebenszyklus oder Filmwiedergabe. Die bestehenden
[Apple-Release-Gates](https://github.com/ralleur/kurtz/blob/kurtz/docs/release/apple-release-plan.md) gelten weiterhin.

## 10. Upstream, Updates und Lizenzen

Upstream-Änderungen werden laufend beobachtet und als überprüfbare
Integrationsänderungen übernommen. Ausgeliefert werden getestete stabile
Kombinationen. Der jeweilige Entwicklungszweig wird nicht automatisch an Nutzer
verteilt; Sicherheitskorrekturen erhalten Vorrang und gegebenenfalls Backports.
Die bestehende kurtz-Automation wird dafür nicht ungefragt geändert.

Der Prozess pro Update:

1. Server, Web, FFmpeg, Transport und relevante Plugins auf Kompatibilität prüfen;
   Unterschiede und nötige Mutti-Anpassungen sichtbar dokumentieren.
2. Upstream-Prüfungen sowie Mutti-Einrichtung, Kopplung, Wiedergabe und Rechte testen.
3. Upgrade auf einer Kopie bestehender Daten und Wiederherstellung mit altem
   Programmstand erproben. Vor Datenbankmigrationen eine passende Sicherung
   erstellen; Medienkopie und Konfigurationssicherung klar unterscheiden.
4. Ein zusammengehöriges Release mit signierten Artefakten, Hashes, Quellen,
   Lizenzinventar und Änderungen erstellen. Erst nach diesen Prüfungen freigeben.

Jellyfin kennt keinen allgemeinen Datenbank-Downgrade. Ein Rückweg kann daher
die Wiederherstellung des vorherigen Datenstands erfordern, nicht nur das
Zurücksetzen der Programmdatei.
[Backup und Restore](https://jellyfin.org/docs/general/administration/backup-and-restore/),
[Upstream-Updatepolitik](https://jellyfin.org/docs/general/testing/upgrades/).

Mutti übernimmt nicht pauschal kurtz' MPL-Lizenz. Jellyfin-Server und -Web führen
GPL-Lizenzen; für den ausgewählten Stand werden Datei-/Komponentenlizenzen,
Laufzeit, FFmpeg, Transport, Fonts und neue Module geprüft. Lizenztexte,
Urheberhinweise und zum ausgelieferten Build passende Quellen werden mitgeführt.
Die Architektur ersetzt diese Prüfung nicht. Quellen:
[Server-Lizenz](https://github.com/jellyfin/jellyfin/blob/master/LICENSE),
[Web-Lizenz](https://github.com/jellyfin/jellyfin-web/blob/master/LICENSE),
[Jellyfin-Markeninformationen](https://jellyfin.org/docs/general/contributing/branding/).

## 11. Reihenfolge und überprüfbare Ergebnisse

Die Etappen beschreiben Ergebnisgrenzen, keine bereits zugesagten Termine.
Eine belastbare Aufwandsschätzung folgt nach dem Transportvorversuch und dem
unveränderten Upstream-Build; gerade diese zwei Unsicherheiten bestimmen den
Umfang wesentlich.

| Etappe | Arbeit und Ergebnis | Abschlusskriterium |
| --- | --- | --- |
| **M0 – Entscheidungen und Techniknachweis** | Mac-/Docker-/NAS-Testmatrix festlegen; passende stabile Upstreams wählen; direkte Verbindung ohne Relay prüfen; Kopplungs-/Control-Architektur und Sicherheitsgrenzen dokumentieren. | Entscheidungsprotokolle und echter direkter WAN-Nachweis; kein versteckter Relay-Verkehr. Offene Grenzen sind benannt. |
| **M1 – Fork und reproduzierbarer Build** | Repositories anlegen, Versionen festlegen, unveränderte Referenz für Mac und Container bauen, CI und Quellen-/Lizenzinventar aufsetzen. | Frische Testinstallationen zeigen Bibliothek und Film; beide Forks und beide Pakete lassen sich aus dokumentierten Quellen bauen. Kann teilweise schon während M0 erfolgen. |
| **M2 – Mutti-Identität und lokale Einrichtung** | Markenpaket, Tokens und Kernansichten; Mac-Hülle und Docker-Paket mit gemeinsamer Weboberfläche; Besitzerzugang und Medienordner. | Mac-Einrichtung ohne Terminal; dokumentiertes Docker-/NAS-Deployment mit anschließendem Webassistenten. Beide benötigen keine manuellen Jellyfin-Konfigurationsdateien. |
| **M3 – Sichere lokale Kopplung** | Schlüssel, QR, Freigabe, Profilrechte, Geräteverwaltung und Wiederherstellung; erste kurtz-Integration. | Zwei frisch installierte Geräte koppeln; abgelaufene/erneut verwendete Einladungen scheitern; Entfernen eines Geräts beendet dessen Zugriff. |
| **M4 – Direkter Fernzugriff** | Vermittlung und STUN produktionsfähig einbinden; Client-Transport vervollständigen; Netzwechsel und Fehlerfälle. | App außerhalb des Heimnetzes spielt über direkten verschlüsselten Weg; gesperrter direkter Weg führt kontrolliert zum Fehler, ohne Relay. |
| **M5 – Betrieb und Beta** | Updates, Backups/Restore und Langzeittests für Mac und Docker/NAS; echte Apple-Clients und Feldtest; Support-/Datenschutztexte. | Die unten stehende Abnahme ist für beide Server-Auslieferungen bestanden und reproduzierbar dokumentiert. |
| **M6 – Gemeinsames erstes Release** | Zusammenpassende Mac-App und Docker-Images, Website, Hilfetexte und Quellen veröffentlichungsbereit machen. | Beide Pakete erfüllen den vereinbarten Umfang. Produkttexte entsprechen den getesteten Plattformen; Veröffentlichung erfolgt im dafür beauftragten Schritt. |

Kritischer Pfad: **M0 → M3/M4 → M5**. Ein lokaler Zwischenstand aus M2 ist
nützlich, darf aber nicht als fertige Fernzugriffslösung ausgegeben werden.
M2 beginnt nach der Basisarbeit aus M1; M3 benötigt die lokale Einrichtung.
Ein großer optischer Umbau des kompletten Upstreams ist kein Vorläufer des
ersten Kopplungstests.

## 12. Abnahme für Day 1

| Bereich | Nachweis |
| --- | --- |
| Einrichtung | Frische Mac-Installation ohne Shell sowie dokumentiertes Docker-/NAS-Deployment mit Webassistent; jeweils erste Bibliothek, Besitzerzugang und Kopplung. Abbruch/Neustart verliert keine bestätigten Einstellungen. |
| Lokaler Betrieb | Wiedergabe und lokale Gerätefreigabe funktionieren bei ausgefallenem öffentlichen Vermittlungsdienst; kein fremdes Netz erhält Zugriff. |
| Echtes WAN | Tests zwischen getrennten Internetanschlüssen, mit öffentlicher IPv4, IPv6, DS-Lite/CGNAT und Mobilfunk. Erwartete Fehlschläge werden dokumentiert, nicht als erfolgreiche Direktverbindung gewertet. |
| Kein Relay | Instrumentierung und gezielte Netzsperren belegen: beim Aufbau, Streaming und Netzwechsel keine Medien über DERP/TURN/Peer-Relay oder Control. Der Test fällt bei impliziten Drittservern durch. |
| Wiedergabe | Reale Bibliothek, mindestens ein längerer Filmtest je freizugebender Clientplattform; Start, Sprung, Resume, HLS/Transcoding, Bilder, Untertitel, Audioauswahl und WebSockets. Bitraten und Hardware dokumentieren. |
| Lebenszyklus | Server-/Client-Neustart, Ruhemodus, abgezogenes Laufwerk und WLAN/Mobilfunk-Wechsel; keine stillen Freigaben oder verlorenen Sperrungen. |
| Rechte | Anderes Gerät/anderer Haushalt, falscher Server, gestohlene oder wiederverwendete Einladung und entfernter Zugriff werden abgewiesen. Aktive Sitzungen enden bei Sperrung. |
| Geheimnisse | QR, Logs, URL-Weiterleitungen, Absturzberichte und Supportexport enthalten keine dauerhaften Zugangsdaten. |
| Updates | Auf Mac und Docker/NAS: Upgrade und Wiederherstellung aus passender Sicherung mit vorhandenen Bibliotheken, Benutzern und Gerätefreigaben; definierte Behandlung alter/geklonter Schlüssel. |
| Gestaltung | Echte Mutti-Ansichten, korrektes Branding, DE/EN, Tastatur, Screenreader, Hell/Dunkel und mobile Freigabe geprüft. |
| Distribution | Mac-Signatur/Notarisierung, Docker-Image-Provenienz und Digests, Hashes, Quellpaket, Lizenzen, unterstützte Systeme und Release-Notizen vollständig. Gemeinsames Komponentenmanifest stimmt mit beiden Paketen überein. |

Für den freiwilligen Feldtest werden technische Ergebnisse ohne Mediennamen
oder Kontoidentitäten ausgewertet: Aufbau erfolgreich/fehlgeschlagen,
Aufbauzeit, Abbrüche, verwendeter direkter Weg und Wiedergabedauer. Es werden
keine Erfolgsquoten aus dem lokalen PoC auf die spätere Nutzerschaft übertragen.

## 13. Day 2 – Relay später gesondert entscheiden

Day 2 beginnt mit einer **neuen Marktprüfung zum damaligen Zeitpunkt**. Das
heutige Cloudflare-Freikontingent oder eine derzeit kostenlose öffentliche
Relay-Infrastruktur wird nicht als Grundlage für die langfristige Kalkulation
festgeschrieben. Die Untersuchung vergleicht:

- vollständig kostenlose Angebote, Freikontingente, Sponsoring sowie selbst
  betriebene Open-Source-Lösungen einschließlich realer Hostingkosten;
- Nutzungsbedingungen für unser Produkt und für Video, Registrierungspflichten,
  Limits, Regionen, Überlastung, Verfügbarkeit und Folgen eines Kontingentendes;
- Anschluss an den bis dahin gewählten Transport, Ende-zu-Ende-Verschlüsselung,
  Metadaten, Geräteautorisierung und Trennung verschiedener Haushalte;
- Kosten pro tatsächlich weitergeleitetem Datenvolumen und Eignung für die
  gemessenen Bitraten, einschließlich hoher Videospitzen;
- Anbieterwechsel, selbst gehostete Alternative und Betrieb ohne Relay bei
  Ausfall oder Abschaltung des Angebots.

Day 1 hält dafür eine kleine interne Transportgrenze und versionierte
Fähigkeitsangaben bereit. Es implementiert weder vorsorglich mehrere
Relay-Anbieter noch einen ungenutzten kostenpflichtigen Dienst. Ein späteres
Relay darf die Gerätefreigabe und die Ende-zu-Ende-Verschlüsselung nicht verändern.

## 14. Startpaket für den Umsetzungsauftrag

Die ersten ausführbaren Aufgaben sind:

1. Testmatrix für die gemeinsam auszuliefernden Mac- und Docker-/NAS-Pakete
   festlegen und Repository-Namen auf Verfügbarkeit prüfen; verbleibende
   technische Entscheidungen mit ihren Gründen dokumentieren.
2. Stabile Server-/Web-Versionen auswählen und Referenz-Builds erzeugen.
3. Den isolierten Vorversuch ohne Relay einschließlich echter WAN-Verbindung
   durchführen und die Transportentscheidung festhalten.
4. Forks und Build-Grundlage in den eigenen Repositories aufsetzen.
5. Mutti-Markenpaket und die erste zusammenhängende Einrichtung gestalten.
6. Für Mac und Docker/NAS dieselbe vertikale Funktionsstrecke liefern: frische
   Mutti-Installation → Medienordner → Besitzer → kurtz-QR → Bestätigung →
   Film im Heimnetz.
7. Dieselbe Strecke über einen direkten Fernzugriff erweitern und die
   Fehlerstrecke ohne möglichen Direktweg gleichwertig fertigstellen.

Der konkrete Fortschritt wird ab Umsetzung in den Mutti-Repositories gepflegt.
Dieses Dokument wandert dann in deren Produktdokumentation; hier bleibt ein
Verweis, damit Client- und Serverplanung verbunden bleiben.
