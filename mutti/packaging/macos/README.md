# Mac-Release signieren und notarisieren

`build-mac.sh` signiert standardmäßig ad hoc (lokale Entwicklung). Ist `MUTTI_SIGN_IDENTITY`
gesetzt, signiert das Skript stattdessen von innen nach außen mit Hardened Runtime und
Zeitstempel: zuerst alle Mach-O-Dateien unter `Contents/Resources` (per `file`-Magic erkannt,
`.dylib` zusätzlich per Namen; nur `server/jellyfin` bekommt `runtime.entitlements`), danach
das Bundle mit `apps/macos/Mutti.entitlements`.
Der Workflow `.github/workflows/mutti-release.yml` nutzt genau diesen Pfad, notarisiert das
Ergebnis und heftet das Ticket an.

## Secrets im Repository

| Secret | Inhalt |
| --- | --- |
| `MUTTI_MAC_CERT_P12` | Base64 des exportierten „Developer ID Application“-Zertifikats (.p12) |
| `MUTTI_MAC_CERT_PASSWORD` | Passwort des .p12-Exports |
| `MUTTI_MAC_SIGN_IDENTITY` | Name der Identität, z. B. `Developer ID Application: Name (TEAMID)` |
| `MUTTI_NOTARY_KEY_ID` | Key-ID des App-Store-Connect-API-Schlüssels |
| `MUTTI_NOTARY_ISSUER_ID` | Issuer-ID aus App Store Connect |
| `MUTTI_NOTARY_KEY_P8` | Inhalt der Datei `AuthKey_<KEY_ID>.p8` (mehrzeilig, unverändert) |

## Zertifikat exportieren

1. In der Apple-Developer-Konsole ein Zertifikat vom Typ „Developer ID Application“ anlegen
   (CSR aus der Schlüsselbundverwaltung) und installieren.
2. Schlüsselbundverwaltung, Kategorie „Meine Zertifikate“: das Zertifikat samt privatem
   Schlüssel markieren und als `.p12` exportieren. Ein Passwort vergeben.
3. Base64 ohne Zeilenumbrüche erzeugen und als Secret hinterlegen:
   `base64 -i DeveloperID.p12 | tr -d '\n' | pbcopy`
4. Den Identitätsnamen für `MUTTI_MAC_SIGN_IDENTITY` nachschlagen:
   `security find-identity -v -p codesigning`
5. In App Store Connect unter „Users and Access > Integrations > App Store Connect API“
   einen Team-Schlüssel mit der Rolle „Developer“ anlegen. Key-ID, Issuer-ID und die
   einmalig herunterladbare `.p8`-Datei ergeben die drei `MUTTI_NOTARY_*`-Secrets.

Der Workflow importiert zusätzlich Apples „Developer ID G2“-Zwischenzertifikat, damit die
Zertifikatskette für den Zeitstempel vollständig ist.

## Workflow starten

GitHub > Actions > „Mutti Release (macOS)“ > „Run workflow“. Als Eingabe die Runtime
wählen (`osx-arm64` oder `osx-x64`). Der Lauf baut das Bundle, signiert es, reicht es bei
Apple ein (`notarytool --wait`), heftet das Ticket an (`stapler`) und lädt
`Mutti-<rid>.zip` als Artefakt hoch. Schlägt die Notarisierung fehl, steht das Apple-Log im
Schritt „Notarize and staple“. Der temporäre Schlüsselbund wird immer gelöscht.

Lokal lässt sich derselbe Pfad testen, wenn die Identität im eigenen Schlüsselbund liegt:
`MUTTI_SIGN_IDENTITY="Developer ID Application: Name (TEAMID)" mutti/packaging/build-mac.sh`

## Was die Notarisierung nicht ersetzt

Notarisierung bestätigt nur, dass Apple in dem Bundle keine bekannte Malware gefunden hat
und alle ausführbaren Dateien korrekt signiert sind. Die übrigen Release-Gates bleiben
Pflicht: grüne Mutti-, Test- und Format-CI, geprüfte `components.lock.json`
(`releaseReady`), Provenance- und Quellenprüfung (`verify-sources.py`, `provenance.py`)
sowie der manuelle Testlauf des Bundles auf einem sauberen Mac.
