# Umsetzungsstand

Stand: 6. Oktober 2026. **Lokale Entwicklungsvorschau; der Gesamtplan ist nicht abgeschlossen.**

Die Forks `ralleur/mutti` und `ralleur/mutti-web` behalten ihre vollständige
Jellyfin-Historie. Die Produktbranches beginnen beim zusammenpassenden stabilen
Stand v12.1. Die Umsetzung liegt zunächst auf `codex/mutti-foundation`; `main`
ist die Review-Basis. Der Web-Commit ist im Komponentenmanifest festgelegt.

## Aktuelle Übergabe und nächste Schritte

Stand 06.10.2026 (abends): P0 (`750f9158a2`), Harness v5 (`c74373310c`) und P1
sind auf `codex/mutti-foundation` zusammengeführt (Merge von
`codex/mutti-p1-content`, Tests grün). kurtz ist auf `codex/rebrand-kurtz` in
thematischen Commits gesichert (letzter `9009779f`, entspricht dem gebauten und
im Simulator geprüften Stand; Zwischencommits nicht einzeln gebaut). Nichts
gepusht, kein Paket ausgeliefert. `qwen3.8:27b-mlx` ist per
Owner-Entscheidung im Katalog „limited“, unter P0 aber ohne Runtime-Attestation
und vertrauenswürdigen Nachweis weiterhin **nicht** produktiv freigeschaltet.

**Plattformentscheidung (Owner, 06.10.2026):** Day 1 ist **Mac only, Apple
Silicon, MLX**. Docker/NAS ist aus dem Entwicklungsplan genommen und steht auf
der [Roadmap](plan.md#17-roadmap-docker-nas-nach-day-1); vorhandener Docker-Code
bleibt unangetastet, wird aber weder gebaut, getestet noch als bereit
bezeichnet. Nachgezogen in `plan.md` (§1, §8, §11/12-Hinweis, §16, §17),
`AGENTS.md` und `contracts.md`; historische Belege bleiben unverändert.

**P1 — gemeinsamer Inhaltsweg: für den Day-1-Umfang umgesetzt und auf dem
Mac-Paket sowie nativ im Simulator geprüft.** Branch `codex/mutti-p1-content`
(Worktree `/Users/ai/workspace/mutti-p1`). Hub: ContentRef mit persistiertem
ID-Index, föderierte Suche `content/search` mit gebundenen Cursorn und ehrlichem
`partial`, Öffnen mit Revisionsstatus und Objekt-Nachprüfung im Medienstrom,
`content/jobs`, Quellen-Provenienz, Rücknahme widerrufener Quellen aus der
Modellhistorie, dauerhaftes Aktionsjournal; Sicherung/Restore umfassen jetzt
ID-Index und Journal. kurtz (`vela-swiftfin`, `codex/rebrand-kurtz`,
Commit `2f32ca98`/`9009779f`): Bereich „Alles“ mit gemeinsamer Suche und Aufträgen, Fotos-/
Dokumentsuche über denselben Weg, Kopplungslink-Korrektur.

Belege: Go-Tests (`-race`) und echte Immich/Paperless-Tests grün; angepasster
`module-package-smoke.py` mit 41 bestandenen Prüfungen über den echten Tunnel;
native Bedienung im iPad-Simulator (Suche, Öffnen, Ausfall, Rechteentzug). Der
Paketlauf deckte fehlende Sicherung von `content-ids.json`/`actions.json` auf
(behoben). Einzelheiten und Grenzen: [P1-Nachweis](evidence/p1-2026-10-06.md);
Vertrag: [contracts.md](contracts.md); Owner-Entscheidungen:
[plan.md](plan.md#offene-entscheidungen-vor-jeweiliger-umsetzung).

**P2 – Laufzeit-Bestätigung, Freigabeeinträge und Harness v6 (06.10.2026,
abends):** Branch `codex/mutti-p2-qualification` (Worktree `mutti-p1`).
Implementiert und mit Unit-Tests geprüft: Attestation der laufenden Bereitstellung
(Engine-Verzeichnis inkl. MLX, Hub-Binary, exaktes Mac-Modell, OS-Build;
externe oder nicht abgeschottete Engine nie freigegeben), signierte
Freigabeeinträge (Ed25519, Schlüssel außerhalb des Repos unter
`~/Library/Application Support/Mutti/release-keys/`, Widerruf), Werkzeug
`mutti-release`, Messbefehl `mutti-hub qualify` auf echten Adaptern,
reproduzierbarer Hub-Build. Owner-Entscheidung: Harness Englisch (v6) mit
Sprachpaketen de/en, Freigabe je Antwortsprache; kurtz sendet die App-Sprache
(`d1f2d6c7`). Englischer synthetischer Korpus in der Testumgebung ergänzt.
Messungen: v6-dev 60/60; Produktsätze real-de-v1 69/84 und real-de-v2 78/84
(jeweils danach gesehen, Lücken behoben); aktuell eingefroren und offen:
real-de-v3 und real-en-v1. **Nichts signiert, keine KI freigeschaltet.**
[Protokoll](evidence/qualification-v6-protocol.md),
[Messungen](evidence/qualification-v6-2026-10-06.md),
[Entscheidungsvorlage Mac-Laufzeit](decision-mac-service-runtime.md).

Nachtrag 06.10.2026, nachts (ohne Inferenz/Builds): Hub-Meldungen folgen der
Sprache der Anfrage (DE/EN; Fehler, Modulstatus, Suchbereiche, Aufträge sowie in
Gesprächen gespeicherte Hinweise und Vorschlagsergebnisse), Test hält jede
Meldung übersetzt; Owner-Webansicht „Freigabe“ (nur lesend, Mutti Web
`6732fe42`, ESLint sauber, TypeScript-/Webbuild noch nicht gelaufen);
Messsonde Wiedergabe mit Rückfall auf progressiven Transcode;
`mutti/tests/qualification-session.py` für einen Messtag. Go-Tests (`-race`) grün.

**Nachtrag 06./07.10.2026, Nacht (ohne Inferenz/Builds; Branch
`codex/mutti-p2-qualification`):**

- **P3 – wiederherstellbare Updates (Mac), implementiert und synthetisch
  geprüft** (`8ab251aa11`, `74f9a6daeb`, `e7cbdb0a18`; `mutti/migrate/update.go`,
  `engine.go`, `http.go`, `update_test.go`): Datenversion, Offline-Snapshot
  vor jedem neuen Build (APFS-Klon, Hashes, atomar sichtbar, Platzprüfung,
  abgebrochene Snapshots werden verworfen), Zustände pending/verified/failed/
  blocked/rolled_back, Downgrade-Sperre, Rollback nur nativ und nur im
  blockierten Zustand. Entscheidung (Plan P3 „Identitäts-/Widerrufsverhalten
  festlegen“): nach der Sicherung entzogene Geräte, Freigaben, Module und
  Kontoverknüpfungen bleiben nach einem Rollback entzogen; danach gekoppelte
  Geräte koppeln neu. Dabei behoben: die Nachprüfung las `connect/state.json`
  statt `connect/connect.json`. Vertrag: [contracts.md](contracts.md#updates-and-rollback-p3-mac).
  **Nicht** im Paket oder mit echten Daten geprüft; offen: native
  Oberfläche für den blockierten Zustand, Wiederherstellung auf leerem
  Zweitziel.
- **P3 – verifizierte Komponentensätze, implementiert und synthetisch
  geprüft** (`427696bcce`): Der Build listet jede Datei unter
  `Contents/Resources` mit SHA-256 (`components.json`, vor `codesign`;
  geprüft: `codesign --deep` verändert dort nichts); Releases signieren die
  Liste (`mutti-release sign-components`, Ed25519). `mutti-migrate` prüft bei
  jedem Start den ganzen Satz (APFS-Klon des echten Pakets: 2.898 Dateien,
  10 Links, 0,35 s); Änderung, fehlende/zusätzliche Datei, fremde Signatur
  oder unsignierter Release-Kanal blockieren vor jeder Datenänderung.
  Release-Schlüssel ist noch keiner hinterlegt (Owner-Schritt); aktuelle
  Pakete sind unsignierte Entwicklungsbuilds.
- **Quell-/Lizenzinventar (M1/P4), offline** (`6a047d1260`):
  `inventory.py` ordnet alle 2.908 Paketdateien zu (0 ohne Zuordnung) und
  liest Lizenzen aus lokalen Metadaten; `notices.py` erzeugt
  `licenses/THIRD-PARTY-NOTICES.txt` (180 Texte); llama.cpp-/mlx-c-Texte zu
  den von Ollama gepinnten Revisionen ergänzt. Beides läuft ab dem nächsten
  Build mit. Offen vor Auslieferung: 6 .NET-Pakete ohne Lizenzangabe,
  Quellpflichten FFmpeg (GPL-3.0+), Server, Web, 9 Prüffälle (u. a.
  CC-BY-SA-4.0). [Nachweis](evidence/license-inventory-2026-10-07.md).
- **Drei unabhängige Reviews, Befunde behoben** (je mit Regressionstests,
  `go test -race` grün): P1 (`920f11a701`), P2 (`51fe393d84`: Modelldateien
  werden auf der Platte geprüft, Bindung je Modell, nur verwaltete Engine,
  Thinking erzwungen, Engine-Digest nach jedem Start veraltet, Links im
  Engine-Ordner), Harness/Bewerter v6 (`3f6b017822`: falsche Korrekturen bei
  Verneinungen, Favoritenfragen, erfolgreicher zweiter Suche, Echo der Frage;
  gruppierte Marken; Summen statt stiller Kappung; Bewerter streng bei
  abgeschnittenen Antworten, Lecks in zurückgezogenen Entwürfen und
  Sprachwechsel in Fremddaten). Offline-Abgleich: alte und neue Erkennung
  stimmen auf allen 468 aufgezeichneten Antworten überein. Die Gate-Sätze
  real-de-v3/real-en-v1 sind weiter eingefroren und ungesehen; die
  Bewerteränderung ist im [Protokoll](evidence/qualification-v6-protocol.md)
  vor ihrem ersten Lauf festgehalten.
- **P4 – DE/EN für Einrichtung und Kopplung, teilweise** (`69203f253f`,
  `d60386ac98`, `e803fc767e`, `b6858afb0b`, `80f38b7f65`): Einrichtungsseite
  (`i18n.js`) und Kopplungsseite übersetzen sich für englische Browser/App;
  Connect-Antworten an Seite, Geräte und Vermittlung folgen
  `Accept-Language`; Mac-App-Texte vollständig in `en.lproj`; Tabelle aller
  Manager-Meldungen (DE als Kennung, zusammengesetzte Meldungen rekursiv)
  samt Tests. Wartungsantworten übersetzt; Zustands-/Update-/Importmeldungen
  des Managers noch nicht verdrahtet (folgt nach dem P3-Review). Swift nur
  typgeprüft, nicht gebaut; Seiten nicht im Browser angesehen.
- **Mac-App: blockiertes Update** (`cd9d66494b`): Die App brach bisher nach
  90 s ohne „ready“ ab, auch während der Sicherung vor einem Update und im
  blockierten Zustand; jetzt Fortschrittsmeldung bzw. Erklärung und Knopf
  für die native Wiederherstellung (mit Bestätigung).
- Folge: Der Harness hat sich nach v6-dev-5 geändert; **die Wirkung auf das
  Modell ist ungemessen.** `qualification-session.py` misst deshalb zuerst den
  v6-Entwicklungssatz und bricht vor den Gate-Sätzen ab, wenn er nicht 60/60
  erreicht.

Nächste Schritte (Inferenz nur tagsüber, Lüfter im Schlafzimmer): ein Aufruf
`python3 mutti/tests/qualification-session.py --testenv
/Users/ai/workspace/mutti/build/module-testenv/private.json --models
/Users/ai/workspace/mutti/build/model-casting/store-mlx/models` baut das Paket
(inkl. Webbuild), misst den v6-Entwicklungssatz (Regression), dann real-de-v3,
real-en-v1 (je mit Wiedergabe leer/unter KI-Last) und den v6-Holdout. Danach
Review/Signatur durch Ralf, Produkt-E2E mit `--expect-qualified`, native
Bedienung der KI in kurtz. Offene Owner-Entscheidungen: Signatur/Review,
Mac-Laufzeit für verwaltete Dienste, Bindung an exaktes Mac-Modell.

**Modellfreigabe v4, 06.10.2026 (P2-Vorarbeit, abgeschlossen als Messung):**
Nutzerauftrag „Werkzeuge/Harness verbessern, dann mit neuem ungesehenem Satz
messen“. Entwickler-Holdout (44 Fälle) **vor** Code-Änderungen eingefroren.
Implementiert (uncommittet): `search_movies` mit Jahr, `watched`/`unwatched`,
„unter“/„höchstens“-Laufzeit, `runtime_desc`; Harness-Korrekturen und Prompt
`mutti-assistant-v4`; Korrekturen als markierte `user`-Nachricht (MLX lehnt
späte `system`-Nachrichten ab); Bewerter vergleicht Filterbedeutung und prüft
`german`. Betroffen: `mutti/hub/{harness,tools,cast}.go`, `harness_test.go`,
`hub_test.go`, `cmd/mutti-cast`, `mutti/tests/casting-v4-{dev,holdout}.py`,
`casting-summary.py`, Fixtures v4. `go test ./...` grün.
Nutzerentscheidung: auf Apple Silicon ab sofort MLX (im [Plan](plan.md#1-verbindliche-entscheidungen-und-planannahmen)
festgehalten; Docker/NAS braucht weiter GGUF).
Ergebnis synthetisch geprüft, Holdout MLX: 9B 123/132, 27B 126/132; **keine
Freigabe** (Nichtwissen 9/12 bei beiden). 27B MLX erfüllt erstmals die
Latenzgrenzen. Bekannte v4-Harness-Mängel (Archivsuche ohne Frage,
Zitier-Korrektur bei „nichts gefunden“) und v5-Schritte:
[Nachweis](evidence/model-comparison-v4-2026-10-06.md),
[Protokoll](evidence/casting-v4-rubric.md). Nächster Schritt dazu: v5 mit diesen
zwei Korrekturen und einem nicht vom Entwickler verfassten Holdout-Satz;
außerdem prüfen, ob die gebündelte Mac-Engine MLX kann.

**Harness v5 für Qwen3.8 27B MLX, 06.10.2026 (P2-Vorarbeit):** Nutzerentscheidung:
Fokus 27B MLX, kein Decider-Modell (im Plan festgehalten). Neuer
Entwickler-Holdout **vor** Code-Änderung eingefroren; v5 behebt die v4-Mängel
(Archivsuche nur bei Fragen, keine Zitier-Korrektur bei „nichts gefunden“,
Promptregel 4). Implementiert und synthetisch geprüft (uncommittet,
`mutti/hub/harness.go`, `harness_test.go`, `cmd/mutti-cast`,
`mutti/tests/casting-v5-*.py`, Fixtures v5), `go test ./...` grün.
27B MLX: Entwicklung 177/180, Regression 129/132, **neuer Holdout 129/132**
(Nichtwissen 12/12, keine ungültigen Marken). Automatische „limited“-Bedingungen
auf dem Holdout erfüllt. Latenz nach Pausieren der störenden VM wiederholt:
Holdout p95 Werkzeugantwort 6,0 s, Entwicklung 8,1 s, Qualität identisch —
Latenzgrenzen erfüllt. **Abschluss, 20:47:** Netzsperre verifiziert (Prüfung
nutzte `nc` ohne `-v`; Sperre selbst wirkte; korrigiert in `engine.go`, Test neu);
gebündelte Engine liefert jetzt MLX-Kernel (`fetch-ollama.py`, `LICENSE-mlx.txt`),
alle drei Sätze damit gemessen: Ergebnisse identisch, Latenz eingehalten trotz
hoher Systemlast; Dialogbewertung durch den Agenten: 3,6 / 3,6 / 3,4. Damit auf
dem Holdout alle „limited“-Bedingungen erfüllt. **Owner-Entscheidung
06.10.2026:** `qwen3.8:27b-mlx` im Katalog „limited“ (`catalog.go`); unter P0
ohne Runtime-Attestation weiterhin keine Produktnutzung. Ollama und MLX in
`mutti/THIRD-PARTY.md`, MLX-Lizenz im App-Paket (`build-mac.sh`). Commit `c74373310c`.
Mac-App daraus vollständig gebaut (`build/macos/osx-arm64/Mutti.app`, 805 MB,
sauberer Baum, MLX-Kernel enthalten, Ad-hoc-Signatur geprüft; .NET SDK 10.0.401
nach `~/.dotnet` installiert). Paket-Smoke mit der Engine aus der App: Holdout
43/44, Netzsperre verifiziert, MLX auf GPU. `module-package-smoke.py` nicht
ausgeführt; App nicht gestartet/bedient.
[Nachweis](evidence/model-comparison-v5-2026-10-06.md),
[Protokoll](evidence/casting-v5-rubric.md). Nächste Schritte: unabhängiger Holdout bzw.
Owner-Dialogbewertung, `module-package-smoke.py` und Bedienung der neuen App, Bewerter-Artefakt (Markdown) als v6-Rubrik,
P0-Runtime-Attestation.

**Session-Vorgaben, 06.10.2026:** [AGENTS.md](../../AGENTS.md) definiert jetzt
Plan-/Statussichtung am Anfang und nachvollziehbare Umsetzung/Übergabe.
[CLAUDE.md](../../CLAUDE.md) importiert dieselbe Quelle für Claude Code; damit
entstehen keine getrennten Arbeitsregeln. Nur Dokumentation geändert.
Importpfad, lokale Dokumentverweise und `git diff --check` geprüft; kein neuer
Agentenprozess gestartet, keine erneute Produktprüfung für diese Textänderung.

| Etappe | Ergebnis | Noch offen |
| --- | --- | --- |
| M0 | Unveränderte Mac-Server- und Web-Referenz gebaut. Zwei echte tsnet-Knoten übertragen lokal verschlüsselt Daten mit ausschließlich STUN; kein DERP-Server läuft. | Getrennte Internetanschlüsse, gesperrtes UDP, CGNAT/IPv6, Netzwechsel; verbindliche Transport-/Control-Entscheidung. |
| M1 | Eigene Forks, Komponenten-Pins, Mac- und Docker-Builds, Entwicklungsanleitung, Sicherheitsregeln und CI implementiert. Lokale API-/Videodatenstrecke auf beiden Paketen bestanden. | Vollständiges Quell-/Lizenzinventar für Distribution, weitere Architekturen und vollständige Wiedergabeabnahme. |
| M2 | Eigene Mutti-Vektormarke, Sora, kurtz-Farben, Web-Assistent; native Mac-Hülle mit Serverstart, Status, Ordnerdialog-Brücke und getrennten Daten; nicht privilegiertes Docker-Paket mit schreibgeschützten Medien. | Native Ordnerauswahl durch alle Dialogschritte, Screenreader/Hellmodus vollständig, echte NAS-Installation; einfache sichere Verwaltung von einem zweiten Gerät. Der derzeitige NAS-SSH-Tunnel ist nur ein Entwicklerweg. |
| M2b | Automatisierter Jellyfin-12.1-Import: Erstwahl, lokale Erkennung, Admin-Anmeldung, interne Online-Sicherung, isolierte Wiederherstellung, Daten-/Dateiprüfung und atomarer Wechsel. Exporthelfer mit einmaligem Transferzugriff vorhanden. | Owner-Test mit echter Bibliothek; große Datenmengen, reale NAS-Mounts, weitere Versionen, externe Plugins/Logins. Automatische Helferinstallation/-entfernung und manueller Archiv-Ausweichweg offen. Siehe [import.md](import.md). |
| M3 | QR-Einladung, TLS-Geräteidentität, bestätigte Profilfreigabe, Keychain-Integration und laufender Widerruf implementiert und lokal geprüft; nativer Neustart/Widerruf besteht in Build 78. | Reale Geräte und vollständige Ablauf-/Bedienabnahme; nach Restore bewusst neu koppeln. |
| M4 | Direkter verschlüsselter Transport und Vermittlungsdienst als Teststand implementiert; siehe unten. Kein Relay. | Öffentlicher Testbetrieb, WAN-Matrix und Wiedergabe bei Netzwechseln. |
| M5 | Lokale Sicherheits-/Integrationstests, konsistente Sicherung, isolierte Probe, Restore auf demselben Paket und begrenzter Wiederanlauf bestehen. | Externe Sicherung/Vollverlust-Restore, Upgrade, Langzeittests, reale Geräte, NAS und zwei echte Anschlüsse. |
| M6 | Lokale Mac-App und Docker-Image verfügbar. | Gemeinsame vollständige Abnahme, Developer-ID/Notarisierung, Quellpakete und freigegebenes Release. |

## Aktuelle Ausgangsbasis und P0, 6. Oktober 2026

[Bestandsabgleich](baseline-2026-10-06.md), [Verträge v1](contracts.md) und
[integrierter Plan P0–P4](plan.md#16-integrierter-ausbauplan-p0p4) sind maßgeblich.
Ältere Tagesabschnitte unten sind historische Prüfstufen, keine widersprüchlichen
aktuellen Funktionszusagen. Insbesondere sind Hub/Adapter und native Modulquellen
vorhanden; deren vollständige Laufzeit-/Paketabnahme bleibt offen.

P0 ergänzt eine standardmäßig sperrende Aufgabenqualifikation. Installierte
Modelle und Besitzerbestätigung reichen nicht zur produktiven KI-Ausführung.
Ohne passenden Nachweis werden Nachrichten/Retry, Run-Start, Werkzeuge und
Bestätigung verweigert. Kein aktuelles Modell ist freigegeben; keine produktiven
Nachweise werden durch Testfixtures ersetzt. [Prüfstand und Grenzen](evidence/p0-2026-10-06.md).

Offen bleiben vor erster Promotion: tatsächliche Engine-/Hardware-Attestation,
vertrauenswürdiger Evidenzimport, englischer Produktionsharness und reale
Aufgabenmessungen. P0 ist eine geprüfte sichere Basis, keine KI- oder Releasefreigabe.
Neue Pakete wurden in P0 nicht ausgeliefert; bestehende laufende Pakete enthalten
noch ihren bisherigen Code. Manuelle Inhalte/Adapter bleiben eigenständige Wege.

## Historischer Ausbau- und Prüfstand vom 5. Oktober 2026

**Aktueller Übergabestand:** [Testpakete und Startanleitung](testpakete-2026-10-05.md),
[gesammelter Foundation-Beleg](evidence/foundation-2026-10-05.md).
Die umfangreiche weitere Implementierung ist dort ausdrücklich von tatsächlich
fehlender Owner-Mitwirkung getrennt. Dieser Stand schließt den Gesamtauftrag nicht ab.

Der neue Auftrag erweitert den bestehenden Produktstand; keine separate Demo.
Aktuell umgesetzt: lokale konsistente Sicherung, Wiederherstellungsprobe und
Wiederherstellung im eigenen Verwaltungsbereich, Profilrechte für Bibliotheken
und Wiedergabe, begrenzter Wiederanlauf nach Serverabsturz. Gerätewiderruf,
Profilsperre und Änderungen an Bibliotheksrechten beenden auch laufende
Transportverbindungen. [Betrieb und Grenzen](maintenance.md).

Mac: vollständiger synthetischer Import einschließlich Intro Skipper, Sicherung,
fehlgeschlagener Zugang ohne Umschaltung, isolierte Probe und echte
Wiederherstellung bestanden. Docker: dieselben Verwaltungsaktionen über die
Produkt-API sowie Neustart nach gezieltem Absturz des eigenen Testservers
bestanden. Die abschließenden Mac-/Docker-Pakete sind nach den letzten Rechteänderungen
erneut gebaut und geprüft; frühere Testpakete enthalten diese Ergänzungen nicht.

Die bestehende kurtz-App (Mac-Build 76) lässt sich inzwischen bedienen; der
früher dokumentierte macOS-Dialog blockiert diesen Lauf nicht mehr. Mit neuer,
rein synthetischer Mutti-Instanz: verschlüsselte Kopplung, Besitzerfreigabe,
Profilanmeldung, Filmansicht, Wiedergabe und 15-Sekunden-Sprung sichtbar geprüft.
Kein WAN- oder physischer iOS-Nachweis. Im neuen Mac-Build 77 führt die Kopplung nach Besitzerfreigabe direkt in das
zugewiesene Profil. Dieser Übergang und die tatsächliche Wiedergabe bestehen.
Die Meldung bei HTTP 403 nennt Profil-/Gerätefreigaben statt technischen Text.
Finaler Build **78** enthält zusätzlich die Reparatur des verwaisten Datenkanals:
Neustart, erneuter Bibliotheksabruf und serverseitiger Widerruf in 19 ms bestanden;
anschließend ist die verständliche 403-Meldung in der nativen Library sichtbar.
Der neue Transporttest weckt blockierte Lese-/Schreibvorgänge ohne Peer auf.

Lokales Modellcasting: drei Kandidaten, je 60 synthetische Fälle mit drei
Wiederholungen, separate native Engine mit OS-Netzsperre und deaktivierter
Cloud. Der bestehende Ollama-Dienst und dessen Modellbestand bleiben unverändert.
Der zusätzliche 4B-Kandidat wurde begrenzt in ein eigenes Testverzeichnis geladen.
Die Ergebnisse zeigen insbesondere unzuverlässige Quellenzuordnung und rechtfertigen
noch keine Modellfreigabe. Zu diesem damaligen Zeitpunkt waren KI/Fotos/Dokumente noch ohne fertiges
Modulbackend; für den späteren Stand gilt der Abschnitt vom 6. Oktober. [Casting-Nachweis](evidence/model-casting-2026-10-05.md).

Immich 2.7.5 und Paperless-ngx 2.20.15 wurden als echte, gepinnte lokale Dienste
ohne Internetzugriff mit synthetischen Daten geprüft: Import/Originale/Suche,
Nutzertrennung, gesperrte Fremdzugriffe und Neustartpersistenz. Paperless-Export
und Restore in eine zweite leere Instanz erhalten Original, Text und Rechte.
Diese Prüfung allein ist eine Dienstqualifikation, kein Nachweis für die später
ergänzten Mutti-Adapter oder nativen Foto-/Dokumentenbereiche. [Ergebnis, Lizenzen und Grenzen](modules-qualification.md).

## Zwischenstand Module, 6. Oktober 2026 (unterbrochen durch Nutzungslimit)

Implementiert und getestet: `mutti/hub` (KI, Fotos, Dokumente; [modules.md](modules.md)),
Anbindung über Connect-Tunnel, Besitzer-Bridge, Manager, Sicherung inkl. Moduldaten,
Mac-Paket mit gepinnter Ollama-Engine (`build/macos/osx-arm64/Mutti.app`), Docker-Image
`mutti:autonomy-20261006`, Mutti-Web-Modulverwaltung, native kurtz-Ansichten
(Fragen/Fotos/Dokumente; Mac Catalyst Build 79 und iOS-Simulator gebaut, **noch nicht
zur Laufzeit bedient**). Hub-Unit-/Race-Tests, echte Immich/Paperless-Integration,
Connect-Weiterleitung, Docker-Modultest (ohne KI) bestanden. Gerätewiderruf meldet
jetzt auch die Jellyfin-Sitzung ab; Linux-Kapazität per statvfs korrigiert.

Casting v2 (720 Antworten, `build/model-casting/2026-10-06-v2`): kein Modell erreicht
die vorab eingefrorenen Schwellen; Qwen 3.6 35B: Werkzeuge 57/60, Dokumente 39/45,
Fremdanweisungen 24/30, Dialog Ø 3,4. v3 mit Werkzeug-zuerst-Wächter läuft/lief
unter `build/model-casting/2026-10-06-v3`; Auswertung offen. Pydantic AI: nur
Paketprüfung (27 Pakete, versteckte Netzpfade `tiktoken`/`genai-prices`), Funktionsvergleich offen.

Nachtrag 6. Oktober: [Freigabetests für die vier Modellachsen](evidence/model-release-tests-2026-10-06.md)
und [neue Messung](evidence/model-comparison-2026-10-06.md) für Qwen3.8 27B
und Qwen3.5 9B liegen vor. Im vorhandenen synthetischen v3-Harness erreicht
27B 177/180, 9B 150/180 automatische Fälle. 27B verfehlt die Latenzgrenzen,
9B die Qualitätsgrenzen; beide bleiben ohne Produktfreigabe. Das gewünschte
Vertrags-Dashboard, der englische Backend-Vertrag und reale Adapter-End-to-End-
Fälle sind als offene Freigabeblöcke erfasst. Die frühere Zeile „v3-Auswertung
offen“ beschreibt den damaligen Zwischenstand.

Offen: Mac-E2E `mutti/tests/module-package-smoke.py`, native Laufzeitprüfung,
Docker-KI-Teil, Produktqualifikation (v3-Vergleich liegt inzwischen vor),
Testpakete und Belegdokumente. Der bisherige positive KI-Pakettest setzt einen
produktiven Pass voraus und ist mit P0 korrekt gesperrt, nicht bestanden.

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
der qualifizierte Import für Jellyfin 12.1 ist jetzt als Teststand vorhanden ([Testanleitung](import.md)).

Die anschließende Owner-Präzisierung setzt einen möglichst automatischen
Ein-Klick-Import als Produktziel: lokal Sicherung intern anstoßen und direkt
lesen, remote einen temporären Umzugshelfer qualifizieren. Das ersetzt die offene
Wahl eines primär manuellen Archivablaufs. Der lokale Normalfall ist umgesetzt;
auf entfernten Servern ist die einmalige manuelle Helferinstallation noch nötig.

## Import-Teststand vom 5. Oktober

Mac arm64 und Linux arm64: vollständige synthetische Übernahme einschließlich
User-IDs/Passwörtern/Rechten, Bibliothek, Playlist, Favoriten, Wiedergabe/Resume
und erhaltenem Quellserver bestanden. Go-Race-Detector und statische Analyse,
Archivpfad-/Unvollständigkeitsprüfungen, Weiterleitungs- und CSRF-/Origin-Schutz
getestet. Exporthelfer gebaut und authentifizierter Transfer geprüft.

Mac-App gebaut und im echten WKWebView geprüft; lokaler Jellyfin wurde automatisch
erkannt. Die tatsächliche Benutzerbibliothek wurde für die Prüfung nicht importiert.
Details und bewusste Grenzen stehen in [import.md](import.md). Kein Release.

Finale Paketprüfung: Docker-Entrypoint mit ausschließlich localhost-veröffentlichten
Verwaltungsports, Browser-Einstieg, Host-/Origin-Abweisung und gesperrter Kopplung
vor Setup bestanden. Profilbild, Anzeigeeinstellungen und Neustartpersistenz
bestehen auf Mac und Linux. Bestehende .NET-Grenztests: 12/12 bestanden.
Der erste native Testbuild aus sauberem Quellstand `755b85c5d0` wurde lokal
signiert und für den Owner geöffnet; der nachfolgende korrigierte Stand steht unten.

### Korrektur nach dem ersten Owner-Test

Der bereits abgeschlossene Preview-Assistent löste eine zusätzliche Anmeldung
als „Mutti-Besitzer“ aus, obwohl beim Jellyfin-Umzug kein weiteres Konto nötig
sein soll. Die Mac-App bestätigt einen Wechsel jetzt nativ über ihren privaten
Prozesszugang; der Anwender gibt nur den bestehenden Jellyfin-Administrator ein.
Browser und Docker prüfen weiterhin den Administrator einer vorhandenen
Zielbibliothek. Quellserver und bisherige Zieldaten bleiben erhalten.

Der vollständige synthetische Mac-Import mit bereits eingerichteter Zielinstanz
und ohne Kenntnis ihres Passworts besteht einschließlich Datenprüfung und
Neustart. Go-Race-Tests, Autorisierungsgrenzen und drei Swift-Tests bestehen.

Aktuelles Testpaket: sauberer Quellstand `5347f116c7`, vollständiger Mac-Neubau
mit verifizierter lokaler Signatur. `build/connect-preview/Mutti.app` ist wieder
geöffnet. Im WKWebView erscheinen ausschließlich die Jellyfin-Zugangsfelder;
der native Wechsel-Dialog und sein Abbruch wurden geprüft. Danach bleibt der
Manager im Zustand `idle` auf dem bisherigen Datenordner. Die echte Bibliothek
wurde nicht importiert. Das neu gebaute Docker-Paket besteht den Entrypoint-Test
inklusive verweigerter nativer Berechtigung für Browseranfragen.

### Intro Skipper als Standard (MK-006)

Intro Skipper 12.0.4.0 gehört zum Mac- und Docker-Paket und wird bei
neuer Einrichtung sowie Import automatisch installiert. Vorhandene Daten dieser
Version werden ohne Plugin-Rückfrage über konsistente SQLite-Snapshots übernommen:
Konfiguration, Ausschlüsse, Segmente und Analysecache. Die Original-DLL wird aus
dem offiziellen Release mit SHA-256-Prüfung paketiert, einschließlich passendem
Quellarchiv und GPL-Lizenz. Der Exporthelfer 0.1.1.0 enthält dieselben Zusatzdaten.

Mac- und Linux-Synthetik: vollständiger Umzug mit vorhandenen Einstellungen und
Sprungmarken, erweiterter Export samt Einmaltickets sowie Neustart bestanden.
Quelle ohne Plugin ebenfalls auf Mac und Linux geprüft. Die Prüfung fordert den
FFmpeg-Funktionsstatus „okay“ des Plugins, nicht nur seinen Installationsstatus.
14 .NET-Tests einschließlich konsistenter WAL-Sicherung, Quellen-Erhalt und
Pfadgrenzen, drei Swift-Tests sowie Go-Race-Tests/statische Analyse bestanden.
Der Restore setzt Hardwarebeschleunigung korrekt auf `none` und bewahrt den
Paket-FFmpeg-Pfad beim internen Neustart. Andere aktive Plugins oder
unqualifizierte Intro-Skipper-Versionen werden weiterhin erkannt.

Finale Pakete aus sauberem Quellstand `2e97113d81`: Mac vollständig gebaut und
lokale Signatur verifiziert; Docker-Entrypoint inklusive automatisch installiertem
Plugin und bisherigen Netzwerk-/Setup-Grenzen bestanden. Der geöffnete Mac-Build
unter `build/connect-preview/Mutti.app` lädt Intro Skipper 12.0.4.0 aktiv und ohne
Chromaprint-Startfehler. Importdialog und lokale Servererkennung geprüft; keine
zusätzlichen Zielkonto-Felder. Der Manager bleibt vor dem Owner-Import auf dem
ursprünglichen Datenordner im Zustand `idle`. Die echte Jellyfin-Bibliothek wurde
für diese Abnahme nicht importiert.

### Importfortschritt und blockierte Quellsicherung (MK-005)

Nach dem Owner-Test zeigt der Import sieben Arbeitsschritte, Gesamt-/Schrittdauer,
beobachtete Sicherungsgrößen und die Zeit seit messbarer Änderung. Nach 90 Sekunden
erscheint ein qualifizierter Wartehinweis. Verbindungsabbruch, 30-Minuten-API-Limit,
45-Minuten-Gesamtlimit und Benutzerabbruch werden unterschieden. Keine geschätzte
Prozentzahl oder Erfolgsmeldung aus bloßen Statusabfragen.

Die reale Quelle blieb im Sperrmodus `Pessimistic` in der Datenbanksicherung
stehen. Mutti fragt diesen Modus jetzt vor dem Backup ab und startet dafür keine
weitere Sicherung. Die vorhandene Quelle wurde für die Diagnose ausschließlich
lesend geprüft; keine Konfiguration geändert und kein Neustart ausgelöst.
Der bisherige Mutti-Datenstand blieb aktiv. Details: [import.md](import.md).

Synthetischer vollständiger Mac-Import, Go-Race-Tests und statische Analyse sowie
Sichtprüfung für Wartehinweis und unterbrochene Verbindung bestanden.

Pakete aus sauberem Commit `2124e7f499`: vollständiger Mac-Build mit verifizierter
Ad-hoc-Signatur sowie Docker-Neubau bestanden. Vollständiger synthetischer
Linux-Import mit Fortschrittsdaten, Intro Skipper, Datenprüfung und Neustart
bestanden; Docker-Entrypoint-/Host-/Origin-/Setup-Grenzen ebenfalls. Die neue
Mac-App unter `build/connect-preview/Mutti.app` wurde erst nach dem Zeitlimit
des alten Imports ausgetauscht und gestartet. Ihre API liefert die neuen
Fortschrittsdaten; der bisherige aktive Datenordner bleibt erhalten. Der
Quellserver benötigt weiterhin die bewusste Sperrmodus-Korrektur und einen Neustart.


### Automatische Quellvorbereitung (MK-005)

Der Import übernimmt jetzt Sicherung und Korrektur des problematischen
Jellyfin-SQLite-Sperrmodus sowie den begleiteten Neustart. Ein Hinweis am
Importknopf und in der bestehenden nativen Wechselbestätigung ersetzt die
manuelle Einstellungssuche. Browser/Docker verwenden ausschließlich den
Quelladministrator; die Mac-App kann zusätzlich einen eindeutig zugeordneten
LaunchAgent des angemeldeten Benutzers auch bei blockierter Anmeldung neu starten.
Serveridentität und Bereitschaft werden vor dem Sicherungsauftrag erneut geprüft.
Originalkonfiguration und ein noch ausstehender Neustart bleiben privat gespeichert.

Synthetisch bestanden: vollständiger Mac-Import nach API-Umstellung/Neustart sowie
nach absichtlich blockierter Datenbanksicherung und Wiederanlauf über launchd.
Der zweite Lauf erhält Benutzer, Playlist, Favoriten, Wiedergabestand, Intro Skipper
und Neustartpersistenz. Go-Race-Tests/statische Analyse prüfen Administrator- und
Dateigrenzen, Konfigurationserhalt, fremde Identität, Wiederaufnahme und fehlenden
Neustartnachweis. Drei Swift-Tests bestanden. Die Erkennung des vorhandenen
Owner-Dienstes wurde ausschließlich lesend geprüft; keine echte Quellkonfiguration
geändert und kein echter Import gestartet.

Paketabnahme: Docker arm64 aus `27a1563a82` besteht den vollständigen synthetischen
Import mit automatischer API-Umstellung und Neustart sowie den Entrypoint-/Host-/
Origin-/Setup-Test. Mac arm64 aus `8c08c7c4d7` enthält denselben Importkern und die
zusätzliche Korrektur einer falschen Portbelegt-Meldung beim schnellen Wiederöffnen
(`TIME_WAIT`). Vollständiger Build aus sauberen Quellen, Ad-hoc-Signatur und drei
Swift-Tests bestanden; aktualisierte App unter `build/connect-preview/Mutti.app`
bereitgestellt. Der ursprüngliche Quelldienst wurde durch den Agenten nicht neu
gestartet; die tatsächliche Migration startet der Owner über den Importdialog.

### Interne Sammlungen bei der Importprüfung (MK-005)

Der Owner-Import erreichte nach mehreren Minuten die Bibliotheksprüfung und
stoppte vor Aktivierung. Die ausschließlich lesende Diagnose zeigte erhaltene
Bibliotheks-IDs und externe Medienpfade. Die interne Sammlung wurde korrekt in
das neue Datenverzeichnis übernommen, aber mit ihrem alten, von Jellyfin in der
API aufgelösten Pfad verglichen. Archivaufbereitung und Bibliotheksprüfung teilen
jetzt dieselbe Pfadzuordnung. IDs, Namen und vollständige Ordnerlisten bleiben
verbindlich; echte Abweichungen benennen die betroffene Bibliothek und Fehlerart.

Eine künstliche Sammlung reproduziert vor der Korrektur dieselbe Fehlermeldung.
Mit der Korrektur besteht der vollständige Mac-Import samt Sammlung/Zuordnung,
Intro Skipper, Benutzerrechten, Wiedergabestand und Neustartpersistenz. Go-Race-
Tests und statische Prüfung bestanden. Die echte Quelle und der fehlgeschlagene
Importordner wurden für die Diagnose nicht verändert.

Paketabnahme aus sauberem Commit `cf343d5353`: Mac vollständig gebaut und lokal
signiert, Docker arm64 neu gebaut. Vollständiger synthetischer Linux-Import mit
Sammlung, Intro Skipper, erhaltenen Daten und Neustart bestanden; Docker-Entrypoint
und Zugriffsgrenzen ebenfalls. Die aktualisierte App unter
`build/connect-preview/Mutti.app` ist geöffnet, der Importdialog erkennt die lokale
Quelle. Der Manager steht auf `idle` mit dem bisherigen aktiven Datenordner.
Der nächste echte Import bleibt beim Owner; keine echte Bibliothek wurde durch
den Agenten importiert oder aktiviert.


### Kurt begleitet den Import (MK-010)

Die beauftragten sieben Animationen ersetzen den Spinner: Schlafen, Po-Rutschen,
Gehen, Laufen, Leckerli, Kotzen, Häufchen. Nach der Zugangsprüfung läuft einmal
Aufwachen/Aufstehen; die echte Verarbeitung wartet nie darauf. Bewegungen wenden
an den Bühnenrändern, Hinterlassenschaften werden je Schleife neu dargestellt.
Die übrigen Fortschrittsdaten bleiben sichtbar. Pause, reduzierte Bewegung,
verlorene Statusverbindung, ausgeblendete Seite und Beendigung halten die Figur an.

Das lokale Paket enthält die unveränderten benötigten Zeichnungen aus Hausers
Kurt-Fassung 11: 127 einzigartige Bildzellen auf zwei verlustfreien WebP-Atlanten,
insgesamt rund 3 MiB einschließlich Effekten und Herkunftsnachweisen. Keine neuen
Bilder erzeugt, keine Hauser-Anwendungslogik übernommen, kein externer Abruf.
Die öffentliche Lizenzierung der privaten Zeichnungen wird damit nicht verändert.

Sieben fokussierte Animationstests und die Go-Importtests bestanden. Die echte
Weboberfläche wurde mit synthetischen Zuständen angesehen: Schlafen, Übergang zum
Sichern, Kotze/Häufchen, Pause und unverändertes Bild, Verbindungsabbruch und Fehler
mit gestoppter Animation. Der Importkern und echte Benutzerdaten wurden dafür
nicht verändert. Mac-/Docker-Paketabnahme folgt für diesen Stand unten.

Paketabnahme aus sauberem Commit `00797474c3`: Mac arm64 vollständig gebaut,
Ad-hoc-Signatur verifiziert, laufende Test-App ausgetauscht und geöffnet. Der
Importdialog erkennt die lokale Quelle; der Manager ist bereit auf dem bisherigen
Datenordner. Das ausgelieferte Kurt-Manifest und ein Atlas wurden über die echte
Mac-API mit dem Quellpaket verglichen. Docker arm64 gebaut; Entrypoint-Test mit
sämtlichen benötigten Clip-/Bildressourcen, Herkunftsgrenzen und gesperrter
Kopplung vor Setup bestanden. Der echte Import wurde nicht vom Agenten gestartet.

### Importlayout und Mutti-Marke (MK-010 / MK-008)

Owner-Korrektur vom 5. Oktober: vertikale Schrittliste links, Kurt rechts in der
freien Fläche; auf kleinen Displays untereinander. Das aktuelle Stadium trägt
die gelbe Markierung. Status und Zeiten stehen direkt unter beiden Spalten.
Kein Eingriff in Importzustände oder Animationstiming.

Die beigefügte Vorlage „03 / Kompakte Bögen“ ist als skalierbare Vektormarke mit
Ringelschwanz, zwei gelben Strahlen und kompakter Wortmarke nachgebaut. Onboarding,
Connect, Bibliothekslogos, Web-Icons und Mac-App-Icon verwenden dieselbe Marke.
Onboarding folgt Graphit `#1F1F1F`, Elfenbein `#FAF8F1`, Gelb `#FFE600` und echten
Sora-Schnitten 400/600/700. Native Bedienelemente bleiben macOS-Standard.

Sichtprüfung mit künstlichem Fortschritt: Desktop, minimales Mac-Fenster und
390-px-Mobilansicht ohne horizontalen Überlauf; die Schrittnamen bleiben jeweils
einzeilig. Laufanimation, markierte erledigte Schritte und Pause geprüft; keine
Browserwarnungen. Die Bibliotheksdaten wurden für diese Änderung nicht angefasst.
Paketabnahme folgt unten.

Paketabnahme aus sauberem Server-Commit `7d07aa9fca` und Web-Commit
`6a67f6eef7`: vollständiger Mac-arm64-Build mit verifizierter Ad-hoc-Signatur
und Docker-arm64-Build bestanden. Der Docker-Entrypoint-Test bestätigt die
verpackten Logos, alle drei Schriftgewichte, Kurt-Ressourcen und bisherigen
Host-/Origin-/Setup-Grenzen. Sieben Animationstests, drei Swift-Tests, Go-Tests
für Import/Connect sowie die gezielte Ressourcen-/Herkunftsprüfung bestanden.

Die vorher beendete Test-App wurde unter `build/connect-preview/Mutti.app`
ausgetauscht und geöffnet; ihre Vorgängerversion liegt als Paketkopie daneben.
Der reale Importdialog ist im neuen Design sichtbar und erkennt die lokale
Jellyfin-Quelle. Ausgelieferte Import-/Connect-Logos und Schriftdateien stimmen
bytegenau mit den Quellen überein. Manager bereit, Phase `idle`; kein echter
Import durch den Agenten gestartet. Die gemeinsame Ideenliste enthält die
Owner-Rückmeldung unter MK-010 und die Markenreferenz unter MK-008.

### Eigene Serververwaltung nach Einrichtung (UX3-Mutti / MK-008)

Owner-Auftrag vom 05.10.2026: Mutti verwaltet den Server; kurtz bleibt der Client.
Die neue Standardoberfläche folgt dem Dashboard-Konzept mit linker Navigation,
kompakter Mutti-Marke, Sora, Graphit/Elfenbein/Gelb und sechs Bereichen:
Übersicht, Bibliotheken, Geräte, Module, Speicher, Einstellungen.

Vorhandene Daten und Aktionen sind echt: Bibliotheken und Eintragszahlen,
Datenträger, Servername, Scanstatus, Bibliotheksanlage, Wiedergabeprofile,
QR-Einladung und vorhandene Connect-Freigaben/Widerruf. Die Besitzeranmeldung
wird wiederverwendet. Mac, Anmeldung und Setup-/Importabschluss führen zur
Serverübersicht. Das native Fenster zeigt keine zusätzliche Medienclient-
Werkzeugleiste. Erweiterte Medien- und Nutzerverwaltung bleiben verlinkt.

Zum damaligen UX3-Prüfstand waren Fotos/Immich, Dokumente/Paperless-ngx,
lokale KI/Ollama und Zuhause **Vorschauen ohne Backend-Anbindung**;
den späteren Hub-Stand beschreibt der Abschnitt vom 6. Oktober. Entwürfe
werden nicht gespeichert und starten keine Dienste. Gemeinsame Modulfreigaben,
Sicherungen und Updates bleiben Ausbauaufgaben. Umfang und Architektur:
[management.md](management.md). Keine neuen externen Dienste oder Abhängigkeiten.

**Prüfstand:** Web `6bb66379eec53599ee7a15fccdbc1109b66adb0d`;
Mac-Paket aus Serverstand `9be0743f0ac197f9c43d7f51989442761218c124`,
beide im Build-Nachweis sauber. Vollständiger Mac-arm64-/Docker-arm64-Build
bestanden; nach der letzten ausschließlich nativen Korrektur Launcher erneut
als Release gebaut und in das unveränderte Server-/Web-Paket übernommen.
Ad-hoc-Signatur erneut verifiziert. Docker enthält denselben Web- und API-Stand;
der letzte Swift-Fix betrifft das Image nicht.

- TypeScript, gezieltes ESLint und Stylelint ohne Fehler; fünf Modelltests
  und fünf Swift-Tests bestanden. API-Build ohne Warnungen/Fehler, Go-Importtests
  bestanden. Die Tests schützen auch normale und abweichende native Herkunfts-
  URLs; Foundation normalisiert `/web/` zu `/web`.
- Frische Docker-Paketprüfung: anonyme/nicht administrative Zugriffe verweigert,
  vorhandener Owner-Zugang für Connect, echte Profilanlage und QR-PNG,
  ablaufende Einladung, begrenzte Aktionsliste, korrekt erhaltene Fehlercodes,
  16-KiB-Grenze auch bei Chunked-Übertragung, fremde Hosts/Origins verweigert.
  Synthetische Bibliothek, Zählung, Speicher, Servername unter Erhalt anderer
  Einstellungen und Scanstatus geprüft. Testcontainer anschließend entfernt.
- Bisheriger Entrypoint-/Onboardingtest einschließlich Branding, Kurt, Host/
  Origin und gesperrter Kopplung vor der Ersteinrichtung bestanden.
- UI im echten Docker-Paket: Owner-Anmeldung zur neuen Übersicht, Bibliothek
  mit leerem Testordner tatsächlich angelegt, QR-Dialog sichtbar, Vorschau-
  Aktivierung ohne Dienststart, Modulkatalog/KI/Einstellungen durchlaufen.
  390-px-Ansicht und Bibliotheksdialog ohne horizontalen Überlauf. Beim Stoppen
  ausschließlich dieses Testcontainers verschwanden alte Kennzahlen zugunsten
  unbekannter Werte und einer eindeutigen Fehlermeldung.
- Finales Mac-Paket unter `build/connect-preview/Mutti.app` geöffnet. Echte
  importierte Bibliotheken und Speicherwerte sichtbar, gespeicherte Anmeldung
  weiter verwendbar. Dialog per Escape geschlossen, Fokus kehrt zum Auslöser
  zurück. Native Fernzugriffseinstellungen geöffnet und ohne Speichern
  abgebrochen. Speicheransicht mit mehreren realen Medienordnern geprüft;
  App auf der Übersicht für den Owner belassen.

Im UI-Test fiel eine vorhandene .NET-/virtiofs-Abweichung auf: Docker Desktop
meldete Kapazitäten um Faktor 256 zu groß. Für `StorageType=Unknown` zeigt die
Oberfläche deshalb keine angeblich verlässliche Kapazität; die zugrundeliegende
virtuelle Messung bleibt ein eigener Backend-Punkt. Native Mac-Datenträgerwerte
werden regulär angezeigt. Keine realen Bibliotheken verändert, kein erneuter
Import gestartet. Vorherige App-Pakete bleiben als Rückfallkopien erhalten.

Reale NAS-Hardware, WAN-Matrix, vollständige VoiceOver-Abnahme und produktive
Auslieferung bleiben gesonderte Gates. Die neue Rechteprüfung im separaten
kurtz-Repo wurde gelesen/ausgeführt; deren laufende Register-/Releasearbeiten
sind keine Freigabe von kurtz-Artefakten durch diese lokale Mutti-Änderung.


### Qualifizierung von Fotos und Dokumenten

Immich 2.7.5 und Paperless-ngx 2.20.15 in eigenen, netzisolierten Testprojekten
mit echten APIs geprüft. Nutzertrennung, Originaldownload und Neustart bestehen;
Paperless zusätzlich Importauftrag, Volltext und separater Export/Restore unter
Erhalt der Eigentümerrechte. [Nachweis und noch fehlende Integration](/Users/ai/workspace/mutti/docs/mutti/modules-qualification.md).
Dies sind keine fertigen Mutti-/kurtz-Module; laufende Originalinstallationen
wurden nicht verändert.
