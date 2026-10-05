# Mutti-Umzugshelfer (Jellyfin 12.1)

Für Jellyfin auf demselben Mac ist kein Plugin nötig, wenn sein Datenordner mit
deinem Mac-Benutzer lesbar ist. Mutti erstellt und liest die Sicherung selbst.

Auf einem anderen Rechner oder bei einem getrennten Container benötigt Mutti
zusätzlich zur Admin-Anmeldung einen Weg zum vollständigen Backup. Der Helfer
stellt diesen Transfer ohne öffentlich abrufbare Backup-URL bereit.

1. Auf dem **Quellserver** im Jellyfin-Pluginordner einen Unterordner
   `Mutti Export_0.1.0.0` anlegen und `Mutti.Export.dll` dort ablegen.
   Der Pluginordner liegt unter Jellyfins Programmdaten (`plugins`).
2. Jellyfin neu starten. Unter Plugins erscheint **Mutti Export**.
3. In Mutti „Aus Jellyfin übernehmen“ wählen, die HTTPS-Adresse und den
   Administratorzugang des Quellservers eingeben. Medienordner müssen auf dem
   Ziel erreichbar sein; bei Bedarf Pfade im Importdialog zuordnen.
4. Nach dem Umzug den Helfer im Jellyfin-Pluginbereich entfernen und Jellyfin
   neu starten. Der Helfer wird nicht in die neue Mutti-Instanz übernommen.

Der Export ist an die erhöhte Admin-Sitzung, ihre Gerätekennung und ein
zufälliges Einmalgeheimnis gebunden. Download ausschließlich über HTTPS oder
lokalen Loopback; keine Weiterleitungen, kein Relay, keine Registrierung.
Download verbraucht die Freigabe; spätestens nach 30 Minuten verfällt sie.
Die normale Jellyfin-Sicherung bleibt am Quellserver für dessen Backupverwaltung.

Die lokale Vorschau installiert dieses Plugin **nicht automatisch** auf fremden
Servern. Ein veröffentlichter, signierter Installationsweg ist noch offen.
Passwörter werden von Mutti weder in Protokolle noch in Einstellungen geschrieben.

Build aus dem Serverrepository:

```sh
dotnet build mutti/export/Mutti.Export.csproj -c Release -p:JellyfinDir=/absolute/jellyfin-12.1-binaries -o build/export
```

GPL-2.0-or-later, wie der Mutti/Jellyfin-Server.
