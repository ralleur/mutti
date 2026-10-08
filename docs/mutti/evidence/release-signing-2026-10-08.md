# Freigabe signiert, Komponentenschlüssel angelegt, qualifiziertes Paket geprüft (08.10.2026)

Quellstand: `codex/mutti-release-signing` auf `codex/mutti-foundation`
`03127c7573` (Integrationsstand vom 07.10., an diesem Tag nach Review
fast-forward übernommen und gepusht). Umgebung: Mac16,9 (Apple M4 Max, 128 GB),
macOS 27.0 (26A428), Xcode 27.0, .NET 10.0.401, Go 1.27.1, lokaler Docker nur
für die isolierte Immich/Paperless-Testumgebung (`module-testenv`). Alle Daten
synthetisch; die laufende Owner-Instanz (`Mutti Preview`, Ports 18595/18596)
wurde nicht berührt.

## 1. Owner-Entscheidungen (08.10.2026, im Chat)

1. Integrationsstand nach Review in `codex/mutti-foundation` übernehmen und pushen.
2. KI-Freigaben für den Kandidaten `7a036fc459` signieren („ich gebe es frei“).
3. Komponentenschlüssel anlegen.
4. Anschließende Agentenarbeit: Paket mit `records.json` bauen, Paket-Smoke mit
   erwarteter Qualifikation, native KI-Prüfung in kurtz, Supervisor im Paket.

## 2. Review und Übernahme des Integrationsstands

Auf `codex/mutti-integration` (`03127c7573`) vor der Übernahme erneut geprüft:
`gofmt -l` leer, `go vet` und `go test -race` grün für `mutti/hub`,
`mutti/migrate`, `mutti/connect`; `swift test` 10/10; `git diff --check` sauber.
`codex/mutti-foundation` wurde fast-forward auf `03127c7573` gesetzt und nach
`origin` gepusht (`b855397f7d..03127c7573`); PR #1 (Draft) zeigt diesen Stand.
Keine Konflikte, kein neuer Merge-Commit.

## 3. KI-Freigabe signiert (P2)

| Schritt | Ergebnis |
| --- | --- |
| Hub-Reproduzierbarkeit | Hub aus dem aktuellen Baum (`go build -trimpath -buildvcs=false`) ist bytegleich zum gemessenen Kandidaten: SHA-256 `812a518260fd6d53…` |
| Kandidaten | `build/qualification-session-20261007-0901/qualification-real-{de-v6,en-v4}/result/candidates.json` (Worktree `mutti-p1`), je 6 Einträge |
| Signatur | `mutti-release sign` mit Schlüssel `mutti-qualification-2026-10` (außerhalb des Repos); Ausgabe `mutti/packaging/qualification/records.json` |
| Verifikation | `mutti-release verify`: zwölf Einträge (de/en × content.assist, media.search, media.read, media.favorite, documents.read, photos.search), alle `passed`, gültig bis 2027-04-05 |
| Bindung | Modell `qwen3.8:27b-mlx` (`5642e974…`), Engine `9dc018e0…`, Hub `812a5182…`, Mac16,9, macOS 26A428, Vertrag `mutti-assistant-v6` je Sprache |

Commit `89737197d0`. Der private Schlüssel wurde weder gelesen noch ausgegeben.

## 4. Komponentenschlüssel angelegt (P3)

`mutti-release keygen --id mutti-release-2026-10` →
`~/Library/Application Support/Mutti/release-keys/mutti-release-2026-10.key`
(0600, außerhalb des Repos). Öffentlicher Schlüssel in `trustedComponentKeys`
(`mutti/migrate/components.go`) eingetragen; `go test -race ./...` für
`mutti/migrate` grün. Runbook `docs/mutti/release-keys.md` auf die tatsächlichen
Dateinamen gebracht. Commit `7084e5d3af`.

## 5. Paket gebaut und verifiziert

`MUTTI_COMPONENT_KEY=… MUTTI_COMPONENT_KEY_ID=mutti-release-2026-10 bash
mutti/packaging/build-mac.sh` auf sauberem Commit `7084e5d3af`
(Log `build/mac-build-2026-10-08-components.log`):

| Prüfung | Ergebnis |
| --- | --- |
| Provenienz | Server `7084e5d3af` sauber, Web `6732fe42cc` sauber, Kanal `local-development`, `releaseReady: false` |
| Komponentenliste | 2.921 Dateien, 0 ohne Zuordnung; `mutti-migrate components verify` → `a574c1d47b1d… signed` |
| Code-Signatur | ad hoc (keine Developer-ID-Identität gesetzt); `codesign --verify --deep --strict` ok |
| `records.json` im Paket | `Contents/Resources/hub/qualification/records.json`, bytegleich mit dem Repo |
| Hub im Paket | SHA-256 `812a518260fd6d53…` = gemessener Kandidat |
| Engine im Paket | `ai-engine/` mit `diff -rq` identisch zum Kandidaten `7a036fc459` |
| Unterschiede zum Kandidaten (erwartet) | Server (47 Dateien), Connect, Manager, Export, Lokalisierung, Lizenzordner, Provenienz, Komponentenlock |

Lizenzinventar unverändert: 6 .NET-Pakete ohne Lizenzangabe, 5 ohne lokalen Text.

## 6. Paket-Smoke mit erwarteter Qualifikation (P1-E2E mit echter KI)

Befehl (aus dem Repo-Root, synthetische Instanz, Modell per `adopt` aus dem
lokalen Teststore `build/model-casting/store-mlx/models`, Digests geprüft):

```
python3 mutti/tests/module-package-smoke.py --app build/macos/osx-arm64/Mutti.app \
  --root build/module-e2e-qualified-N --testenv build/module-testenv/private.json \
  --model qwen3.8:27b-mlx --model-source adopt \
  --adopt-from build/model-casting/store-mlx/models --expect-qualified
```

- **Lauf 1** (`build/module-e2e-qualified-1`): 38 Prüfungen bestanden, darunter
  Komponentenliste `signed` in der Datenversion, Netzsperre der Engine, AT-04
  (echter Werkzeugaufruf über Profilrechte, beide erwarteten Filme) und AT-03
  (Streaming und Werkzeugstatus). Abbruch bei **AT-06**: Das Modell antwortete
  „… Laufzeit von **1 Minute 35 Sekunden** [Q2]“, die Prüfung verlangte die
  Zeichenkette „95“. Produktverhalten korrekt, Prüfung zu eng. Korrektur im
  Testskript: AT-06 akzeptiert „95“, „1:35“ und „1 Minute 35“/„1 Min. 35“.
  Hub und Paket unverändert.
- **Lauf 2** (`build/module-e2e-qualified-2`, `result.json` `passed: true`):
  **63/63 Prüfungen bestanden, 0 fehlgeschlagen.** Damit erstmals die positive
  KI-Strecke im echten Paket: Komponentenliste `signed`, Modell per `adopt`
  gegen den gepinnten Digest geprüft (20 s), Engine in der macOS-Netzsperre,
  AT-02 Modulsicht je Profil, AT-03 Streaming/Werkzeugstatus, AT-04 echter
  Jellyfin-Werkzeugaufruf über Profilrechte (Nordlicht, Sommer am See; kein
  privater Film), AT-06 Rückfrage auf den zweiten Treffer, AT-07 PDF-Anhang
  lokal extrahiert und korrekt beantwortet (128,40 EUR, 15.11.2026, Quelle Q3),
  AT-09 Abbruch in 0,02 s und genau ein Retry, AT-10 keine Fremdlese (Beta),
  AT-11/AT-16 Favorit nur als Vorschlag, fremde Bestätigung abgewiesen,
  bestätigte Aktion nachgelesen, Sicherung mit Gesprächen und Restore-Probe,
  CO-06 Rechteentzug sofort wirksam, CO-07 Ausfall ehrlich als `partial`,
  AT-12 Widerruf beendet den Zugriff während einer laufenden Antwort.
  Antwortsprache de; die en-Freigabe ist über `records.json` enthalten, der
  Smoke stellt nur deutsche Fragen.

## 7. Native KI-Prüfung in kurtz (iPad-Simulator)

- **Lauf 3** (`build/module-e2e-qualified-3`, `--keep`, `MUTTI_E2E_ICE=lo0,en0`):
  62/62 Prüfungen bestanden (AT-12 entfällt bei `--keep`), Instanz gehalten.
- kurtz: `vela-swiftfin`, Branch `codex/rebrand-kurtz`, Commit `d1f2d6c7`
  (sauberer Baum), Debug-Build für den iOS-Simulator (`xcodebuild … -scheme
  Swiftfin`, BUILD SUCCEEDED). Gerät: iPad Pro 13-inch (M5), **iOS 27.0**
  Simulator. Das in P1 genutzte iOS-26.5-Gerät war während des Laufs von einer
  anderen Automatisierung belegt (Hauser-App wurde dort fremd gestartet) und
  wurde deshalb nicht weiter verwendet. Bedienung über `idb`
  (`describe-all`/`tap`/`text`) und `simctl` (Screenshots).
- Ablauf: Einladungslink per `module-native-pairing.py invite` →
  `simctl openurl` → Systemdialog „In kurtz öffnen?“ → Kopplungsblatt mit
  vorbefülltem Link → „Sicher verbinden“ → Besitzerfreigabe
  (`module-native-pairing.py approve`, Profil Alpha) → Server „Mutti Module E2E“
  wählen → „Benutzer hinzufügen“ (gerätegebundene Anmeldung ohne Passwort).
- Sichtbar geprüft:

| Schritt | Ergebnis |
| --- | --- |
| Mediathek nach Anmeldung | Bereiche Bibliotheken / **Alle** / Filme / TV Serien / **Fotos** / **Dokumente** und das Funkeln-Symbol „Fragen“ ([Bild](release-signing-2026-10-08/01-mediathek-mit-fragen.png)); über das Gateway des Simulators liefert `capabilities?language=de` alle Module `ready`, KI-Modell `qwen3.8:27b-mlx` |
| „Fragen“ | serverseitiger Verlauf des Paketlaufs für Alpha: Suche, Rückfrage, Rechnungsanhang mit Quelle Q3, abgebrochene lange Antwort mit „Erneut versuchen“, bestätigter Favoriten-Vorschlag „Erledigt.“ ([Bild](release-signing-2026-10-08/02-fragen-verlauf.png)) |
| Neue Frage „Welche Filme habe ich schon gesehen?“ | Werkzeugstatus „1 Filme gefunden“, Antwort „Du hast einen Film gesehen: **Schon gesehen** (2023) – 1 min 28 s [Q4]“ mit Quellenkarte; fertig nach rund 20 s ([Bild](release-signing-2026-10-08/03-native-antwort.png)) |
| Nach dem Supervisor-Neustart (Abschnitt 8) | Gateway des Clients liefert weiter `Users/Me` 200 und `capabilities` mit `ai`/`content` `ready` |

Hinweise: `idb ui text` tippte das Fragezeichen auf der deutschen
Simulator-Tastatur als „_“; das Modell beantwortete die Frage trotzdem korrekt.
Die Werkzeugleisten-Schaltflächen (Fragen, Dateien) tauchen in `idb
describe-all` nicht auf; getippt wurde per Koordinate aus dem Screenshot.

## 8. Supervisor im Paket

Gegen die gehaltene Instanz aus Lauf 3 (Manager `mutti-migrate` aus dem
gebauten Paket, Port 32594). Skript: `kill -9` auf das Jellyfin-Kind der
Instanz (PID aus `pgrep -f "server/jellyfin --datadir <root>/server"`),
dann `GET /api/state` alle 0,5 s; Ergebnis
`build/module-e2e-qualified-3/supervisor-check.json`.

| Zeit | Zustand |
| --- | --- |
| 0,0 s | `complete`, `ready`, `restarts 0`, Connect `running` |
| 1,0 s | `restarting`, Meldung „Der Server wurde unerwartet beendet. Mutti startet ihn neu …“, `restarts 1`, Connect leer |
| 9,1 s | wieder `complete`/`ready`, vorherige Meldung kehrt zurück, Connect `starting` |
| +70 s | `restarts 0`, Connect `running`, keine Servicemeldung |

Alle sieben Prüfungen bestanden: Neustarthinweis, Bereitschaft binnen 180 s,
neue Jellyfin-PID (alte weg), `/System/Info/Public` 200 nach dem Neustart,
Connect läuft wieder, Zähler nach 60 s auf 0, keine Servicemeldung. Der
gekoppelte Simulator-Client erreichte Server und Hub danach weiter
(Abschnitt 7). Nicht geprüft: Aufgeben nach erschöpftem Backoff, „Paket
ersetzt“, Anmeldeobjekt und Wachhalten der Mac-App (Abschnitt 9).

## 9. Grenzen

- Ad-hoc-Signatur: Ein Developer-ID-signiertes Release verändert Hub- und
  Engine-Digest; die hier signierte Freigabe gilt nicht für dieses Paket. Dafür:
  signiert bauen, auf genau diesem Paket messen, dann signieren (Plan §18).
- Synthetische Daten, ein Mac, ein Simulator-Gerät; keine physischen Geräte,
  kein WAN.
- Anmeldeobjekt und Wachhalten der Mac-App wurden nicht geprüft: Die App-Oberfläche
  nutzt die Owner-Daten (`Mutti Preview`), deren Instanz lief; ein zweiter Start
  derselben App kollidiert damit.
- Kanal `local-development`; keine Notarisierung, keine Auslieferung.
