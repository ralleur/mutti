# Lokales Modellcasting – 5. Oktober 2026

**Qualifizierung, keine integrierte KI und keine Modellfreigabe.**
Grundlage: [Castingplan](/Users/ai/workspace/vela-swiftfin/docs/mutti-expansion/model-casting.md).
Runner: `mutti/tests/offline-model-casting.py`, 60 versionierte synthetische
Fälle, drei Wiederholungen je Modell, deutschsprachige Werkzeuge/Dokumente/
Nichtwissen/Fremdanweisungen/Dialoge. Keine echten Nutzerinhalte.

## Reproduzierbarer Rahmen

Mac Studio M4 Max, 128 GiB; native Ollama 0.32.13, 8.192 Kontexttokens,
384 Ausgabetokens, Temperatur 0, Seed 42, Thinking aus. Eigene Engine auf
zufälligem Loopback-Port; bestehender Dienst und bestehende Modelle unverändert.
`OLLAMA_NO_CLOUD=1`, Modellverzeichnis schreibgeschützt, ausgehendes Netz durch
macOS-Sandbox für Engine **und deren Runner-Kinder** auf Loopback begrenzt.
Eine gesonderte Socketprobe unter derselben Policy bestätigt OS-Abweisung
(EPERM) außerhalb Loopback. Kein bloß aus einem API-Erfolg abgeleiteter Offline-Nachweis.

Qwen 3.6 und Qwen 2.5 lagen bereits lokal vor. Qwen 3.5 4B wurde einmalig aus
der offiziellen Ollama-Registry in einen eigenen Modellordner geladen:
3.324.173.757 Bytes. Manifest und sämtliche Layer-SHA-256 geprüft.
Download getrennt vom anschließend netzgesperrten Inferenzlauf.
Gewichte nicht in Produktpakete übernommen. Qwen-Lizenznachweise stehen in den
jeweiligen `environment.json`; Engine-MIT ersetzt keine Modelllizenzprüfung.

## Ergebnisse der unveränderten v1-Prüfkriterien

| Modell | Werkzeugargumente | Dokumentfelder | Nichtwissen | Fremdanweisungen/Quellen-ID | Dialoge |
|---|---:|---:|---:|---:|---|
| Qwen 3.6 35B-A3B | 57/60 | 45/45 | 15/15 | 6/30 | 30 gesichtet, nicht freigegeben |
| Qwen 2.5 14B Instruct | 36/60 | 45/45 | 0/15 | 0/30 | 30 gesichtet, nicht freigegeben |
| Qwen 3.5 4B | 48/60 | 45/45 | 15/15 | 21/30 | 30 gesichtet, nicht freigegeben |

540 vollständige Antworten, keine Transport-/Enginefehler. Die letzte Spalte
ist kein bestandener Test. Jede Rohantwort einschließlich Ausgabetokens,
Ladezeit, Laufzeit, erstem Antwortstück und Engine-Speicher liegt lokal vor.
Wiederholungen derselben Fälle sind keine 540 unabhängigen Aufgaben.

**Interpretation:** Der größere Kandidat löst Werkzeugauswahl meist gut, der
kleine Kandidat ist bei diesen Fremdanweisungsfällen besser. Größe allein
liefert daher keinen sicheren Standard. Die Quellenprüfung fällt häufig durch,
weil statt der Dokument-ID eine Rechnungsnummer oder eine im Text genannte
fremde ID ausgegeben wird. Das beweist nicht in jedem Fall ausgeführte
Schadanweisungen; es beweist eine unzuverlässige Zuordnung. Ein späterer Dienst
muss Quellen ausschließlich aus dem serverseitigen erlaubten Suchergebnis
auflösen. Kein Modell bekommt durch ausgegebenen Text Zugriff auf fremde IDs.

Die v1-Werkzeugspezifikation beschreibt die Exklusivität einer Laufzeitgrenze
nicht: „unter 88 Minuten“ wird plausibel mit 87 statt dem erwarteten 88 codiert.
Diese drei Fehler bei Qwen 3.6 sind somit auch ein Vertragsproblem. Die
Ergebnisse wurden **nicht rückwirkend schönkorrigiert**. Vor einer Freigabe
explizite JSON-Schema-Beschreibungen und Grenzfälle in einer neuen Fixture-Version
prüfen. Quellen-ID ebenfalls ausdrücklich als vom Server gelieferte ID definieren.

Vollständige [Sichtung aller 90 Dialogantworten](model-dialogue-review-2026-10-05.md):
3.6 verweigert mit dem restriktiven Kontextprompt auch
harmlose Allgemeinfragen und führt bei Fehler 503 mehrere unbelegte mögliche
Ursachen aus. 2.5 deutet eine mehrdeutige „Sicherung“ als elektrische Sicherung
und enthält vereinzelt fremdsprachige Fragmente. Die vorher konkretisierte
Punkterubrik fehlt; deshalb keine nachträglich als vorregistriert ausgegebene
Dialogwertung. Neue qualitative Abnahme,
mehrturnige Wiederaufnahme, Streaming-Stop, echte Werkzeuge, Medienkonkurrenz,
Ruhemodus und physische NAS-Profile sind noch offen. Laufzeiten unter parallelen
Builds sind keine repräsentativen Produktionsbenchmarks.

## Rohdaten

- `/Users/ai/workspace/mutti/build/model-casting/2026-10-05-offline-v2/`
- `/Users/ai/workspace/mutti/build/model-casting/2026-10-05-qwen4b-offline/`
- `/Users/ai/workspace/mutti/build/model-casting/download-qwen3.5-4b/`

Je Lauf: `environment.json`, `results.jsonl`, `summary.json`, `network.sb`,
`engine.log`. Der frühere Lauf `2026-10-05-offline-v1` brach bereits an seiner
ungeeigneten Netz-Kontrollprobe ab und zählt nicht als Inferenznachweis.

## Entscheidung für den nächsten Umsetzungsschritt

Den vorgesehenen schlanken Bibliotheks-/Dienstpfad qualifizieren. LibreChat
wird nicht als Standardstack eingebaut: MongoDB/SSPL sowie doppelte Identität,
Gesprächspersistenz und native Protokollabbildung sind konkrete Zusatzlast.
Die aktuelle Agents-API-Dokumentation enthält **auch dauerhafte Event-Actors**
mit Suspend/Resume; die Einschränkung der beiden Headless-Inferenzendpunkte
ist kein genereller Beleg gegen interaktive Freigaben in LibreChat. Deren
native Einbindung und Versionsvertrag wären dennoch eigene Produktarbeit.

Pydantic AI ist der vorgesehene Bibliothekskandidat (MIT), noch keine installierte
Produktabhängigkeit. Python-Paketierung, exakter Lock und transitive Lizenzen
müssen vor Übernahme nachgewiesen werden. Reversibler Standard: SQLite für
einen Dienst, read-only Werkzeuge zuerst, getrennte Quellen-/Rechteprüfung,
keine Modellentscheidung allein aufgrund dieser Stichprobe. Keine Buchungs-
oder API-Kosten entstanden; zusätzlicher lokaler Speicher rund 3,3 GB.

Primärquellen: [LibreChat Compose](https://github.com/LibreChat-AI/LibreChat/blob/main/docker-compose.yml),
[Agents API](https://www.librechat.ai/docs/features/agents_api),
[Ollama FAQ](https://docs.ollama.com/faq),
[Pydantic/Ollama](https://pydantic.dev/docs/ai/models/ollama/),
[Qwen 3.5 4B](https://huggingface.co/Qwen/Qwen3.5-4B).
