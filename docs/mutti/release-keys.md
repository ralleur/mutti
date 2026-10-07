# Release-Schlüssel (Owner-Ablauf)

Zwei Ed25519-Schlüssel, beide **außerhalb des Repositorys** unter
`~/Library/Application Support/Mutti/release-keys/` (Rechte `0600`). Im Code
stehen nur die öffentlichen Schlüssel mit Kennung. Ein privater Schlüssel
wird nie committet, in Logs ausgegeben oder in einen Chat kopiert.

| Zweck | Kennung (Beispiel) | Öffentlicher Schlüssel im Code | Signiert |
| --- | --- | --- | --- |
| KI-Freigabeeinträge | `mutti-qualification-2026-10` (vorhanden) | `trustedReleaseKeys` in `mutti/hub/attestation.go` | ausgewählte, geprüfte Kandidaten einer Messung |
| Komponentenliste des Pakets | z. B. `mutti-release-2026-10` (**noch nicht angelegt**) | `trustedComponentKeys` in `mutti/migrate/components.go` | `Contents/Resources/components.json` jedes Release-Pakets |

## Komponentenschlüssel anlegen (einmalig, Owner)

```
cd mutti/hub
go run ./cmd/mutti-release keygen --id mutti-release-2026-10 \
  --out "$HOME/Library/Application Support/Mutti/release-keys/components.key"
```

Die Ausgabe nennt Kennung und öffentlichen Schlüssel. Den öffentlichen
Schlüssel in `trustedComponentKeys` eintragen und committen; erst danach
kann ein Paket als signiert gelten. Ohne Eintrag blockiert ein signiertes
Paket den Start („unbekannter Schlüssel“), damit nie ein fremder Schlüssel
still akzeptiert wird.

## Release-Build signieren

```
MUTTI_COMPONENT_KEY="$HOME/Library/Application Support/Mutti/release-keys/components.key" \
MUTTI_COMPONENT_KEY_ID=mutti-release-2026-10 \
bash mutti/packaging/build-mac.sh
build/macos/osx-arm64/Mutti.app/Contents/Resources/migrate/mutti-migrate \
  components verify build/macos/osx-arm64/Mutti.app/Contents/Resources
```

`verify` muss `signed` melden. Ein Paket mit Kanal `release` in
`build-provenance.json` startet nur mit gültiger Signatur; Entwicklungs-
builds (`local-development`) laufen unsigniert und werden so vermerkt.

## KI-Freigaben signieren

Nur nach Review der Messung und ausdrücklichem OK des Owners:

```
go run ./cmd/mutti-release sign --key ".../release-keys/<qualification>.key" \
  --id mutti-qualification-2026-10 --candidates <messung>/result/candidates.json \
  --out mutti/packaging/qualification/records.json [--tasks …] [--languages de,en]
go run ./cmd/mutti-release verify --file mutti/packaging/qualification/records.json
```

## Verlust, Kompromittierung, Wechsel

Neuen Schlüssel mit neuer Kennung anlegen, öffentlichen Schlüssel ergänzen,
den alten Eintrag entfernen und neu bauen. Pakete mit dem alten Schlüssel
starten danach nicht mehr bzw. ihre KI-Freigaben gelten nicht mehr. KI-
Freigaben lassen sich zusätzlich einzeln widerrufen (Widerrufsliste im
Freigabedokument). Datensicherung der Schlüssel: verschlüsselt und getrennt
vom Mac (z. B. Passwortmanager); ohne Sicherung bleibt nur der Wechsel.
