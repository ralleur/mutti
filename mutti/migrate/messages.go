// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// englishMessages translates the manager's user-facing texts. The German
// text is the message ID (a fmt format for composed messages, as written in
// the code); the manager keeps working with German strings and translates
// only when it answers a request in English. Placeholders are translated
// recursively, so a German reason inside a message is translated as well;
// technical details from other components stay as they are.
// TestEveryManagerMessageHasAnEnglishText keeps the table complete.
var englishMessages = map[string]string{
	"%w Die bisherigen Einstellungen sind gesichert; es wurde keine weitere Sicherung gestartet.": "%s The previous settings are backed up; no further backup was started.",
	"Abschließend geprüfte Sicherung": "Backup verified in the final check",
	"Alte Gerätesitzungen und API-Schlüssel werden nicht wiederhergestellt. Geräte bitte neu koppeln.": "Old device sessions and API keys are not restored. Please pair devices again.",
	"Alte Gerätesitzungen und API-Schlüssel werden nicht übernommen; Geräte neu koppeln.":              "Old device sessions and API keys are not taken over; pair devices again.",
	"An dieser Adresse antwortet ein anderer Jellyfin-Server. Der Import wurde angehalten.":            "A different Jellyfin server answers at this address. The import was stopped.",
	"Außerhalb des Docker-Pakets ist ausschließlich Loopback erlaubt.":                                 "Outside the Docker package only loopback is allowed.",
	"Benutzer, Bibliotheken und Wiedergabestand werden in Mutti wiederhergestellt …":                   "Restoring users, libraries and playback progress in Mutti …",
	"Benutzerzugänge, Bibliotheken und Intro Skipper werden geprüft …":                                 "Checking user accounts, libraries and Intro Skipper …",
	"Bitte Einrichtung oder laufenden Vorgang abwarten.":                                               "Please wait for the setup or the running operation to finish.",
	"Bitte Sicherung und Benutzerzugang prüfen und den Vorgang bestätigen.":                            "Please check the backup and the user account and confirm the operation.",
	"Bitte alle Felder prüfen.": "Please check all fields.",
	"Bitte bestätigen, dass du zu einer importierten Bibliothek wechseln möchtest. Der bisherige Mutti-Datenstand bleibt erhalten.": "Please confirm that you want to switch to an imported library. Your existing Mutti data is kept.",
	"Bitte den Import mit automatischer Vorbereitung starten. Jellyfin muss dafür kurz neu gestartet werden.":                       "Please start the import with automatic preparation. Jellyfin has to restart briefly for this.",
	"Bitte den automatischen Import verwenden. Archivdateien werden nur nach Quellenprüfung angenommen.":                            "Please use the automatic import. Archive files are only accepted after the source has been verified.",
	"Bitte die Seite neu öffnen.":                                          "Please reopen the page.",
	"Bitte eine gültige Serveradresse ohne Zugangsdaten eingeben.":         "Please enter a valid server address without credentials.",
	"Bitte einen fertig eingerichteten anderen Jellyfin-Server auswählen.": "Please choose another Jellyfin server that is fully set up.",
	"Bitte mit dem Administrator der bestehenden Bibliothek anmelden oder den Import direkt in der Mutti-Mac-App bestätigen. Ein zusätzliches Mutti-Konto wird nicht benötigt.": "Please sign in with the administrator of the existing library or confirm the import directly in the Mutti Mac app. No additional Mutti account is needed.",
	"Bitte zuerst den Server beenden.":                 "Please stop the server first.",
	"Container-Modus benötigt eine Docker-Umgebung.":   "Container mode requires a Docker environment.",
	"Container-Modus ist nur im Docker-Paket erlaubt.": "Container mode is only allowed in the Docker package.",
	"Das Mutti-Paket ist beschädigt oder verändert (%s). Bitte Mutti neu installieren. Es wurde nichts verändert.":                                                 "The Mutti package is damaged or modified (%s). Please reinstall Mutti. Nothing was changed.",
	"Das Update konnte nicht abgeschlossen werden (%s). Der Datenstand vor dem Update ist gesichert; die vorherige Mutti-Version bietet die Wiederherstellung an.": "The update could not be completed (%s). The pre-update data is backed up; the previous Mutti version offers to restore it.",
	"Das mitgelieferte Intro Skipper fehlt. Bitte das vollständige Mutti-Paket verwenden.":                                                                         "The bundled Intro Skipper is missing. Please use the complete Mutti package.",
	"Das mitgelieferte Intro Skipper konnte nicht geladen werden. Der bisherige Datenstand bleibt aktiv.":                                                          "The bundled Intro Skipper could not be loaded. Your previous data stays active.",
	"Das zusätzliche Plugin „%s“ benötigt eine geprüfte Übernahme. Der Import wurde vor der Änderung gestoppt.":                                                    "The additional plugin “%s” needs a verified import. The import was stopped before anything changed.",
	"Datei ist zu groß.":                               "The file is too large.",
	"Dateipfad konnte nicht eindeutig geprüft werden.": "The file path could not be verified unambiguously.",
	"Datenstand vor dem Update wiederhergestellt. Der zuvor verwendete Stand liegt in %s.":                                                                                                                     "Pre-update data restored. The previously used state is in %s.",
	"Datenstand vor dem Update wiederhergestellt. Der zuvor verwendete Stand liegt in %s. %d nach der Sicherung entzogene Geräte oder Freigaben bleiben entzogen; danach gekoppelte Geräte bitte neu koppeln.": "Pre-update data restored. The previously used state is in %s. %d devices or permissions revoked after the backup stay revoked; please pair devices paired afterwards again.",
	"Deine Jellyfin-Bibliothek ist jetzt in Mutti bereit.":                                                                                                                                                     "Your Jellyfin library is now ready in Mutti.",
	"Der Datenstand wurde bereits von %s geöffnet. Diese Version darf ihn nicht verwenden. Bitte %s oder neuer verwenden oder die Sicherung vor dem Update wiederherstellen.":                                  "The data has already been opened by %s. This version must not use it. Please use %s or newer, or restore the pre-update backup.",
	"Der Jellyfin-Dienst hat sich geändert. Der Neustart wurde angehalten.":                                                                                                                                    "The Jellyfin service has changed. The restart was stopped.",
	"Der Medienordner %s ist hier nicht erreichbar. Ordnerzuordnung ergänzen und erneut versuchen.":                                                                                                            "The media folder %s is not reachable here. Add the folder mapping and try again.",
	"Der Name der Bibliothek %q hat sich geändert.":                                                                                                                                                            "The name of the library %q has changed.",
	"Der Server hat die Anfrage abgelehnt (HTTP %d).":                                                                                                                                                          "The server rejected the request (HTTP %d).",
	"Der Server hat innerhalb von 30 Minuten nicht geantwortet. Eine angeforderte Sicherung kann auf Jellyfin weiterlaufen. Bitte dort den Status prüfen, bevor du erneut startest.":                           "The server did not answer within 30 minutes. A requested backup may continue on Jellyfin. Please check its status there before you start again.",
	"Der Server ist nicht erreichbar. Adresse und Verbindung prüfen.":                                                                                                                                          "The server is unreachable. Check the address and the connection.",
	"Der Update-Nachweis ist beschädigt.":                                                                                                                                                                      "The update record is damaged.",
	"Der aktuelle Datenstand konnte nicht beiseitegelegt werden; der bisher verschobene Teil liegt in %s.":                                                                                                     "The current data could not be set aside; the part moved so far is in %s.",
	"Der bestehende Benutzerzugang funktioniert in der importierten Instanz nicht. Es wurde nicht umgeschaltet.":                                                                                               "The existing user account does not work in the imported instance. Nothing was switched.",
	"Der letzte Wartungsvorgang wurde durch einen Neustart unterbrochen. Bitte den aktiven Datenstand prüfen, bevor du erneut startest.":                                                                       "The last maintenance task was interrupted by a restart. Please check the active data before you start again.",
	"Der lokale Jellyfin-Dienst hat sich geändert. Es wurde nichts verändert.":                                                                                                                                 "The local Jellyfin service has changed. Nothing was changed.",
	"Der lokale Jellyfin-Dienst ist nicht eindeutig zugeordnet. Es wurde nichts verändert.":                                                                                                                    "The local Jellyfin service cannot be identified unambiguously. Nothing was changed.",
	"Der lokale Jellyfin-Dienst ist nicht mehr verfügbar.":                                                                                                                                                     "The local Jellyfin service is no longer available.",
	"Der lokale Jellyfin-Dienst konnte nicht beendet werden.":                                                                                                                                                  "The local Jellyfin service could not be stopped.",
	"Die Abschlussprüfung sucht nach Änderungen während des Umzugs …":                                                                                                                                          "The final check is looking for changes made during the move …",
	"Die Aktivierung ist fehlgeschlagen. Der bisherige Mutti-Datenstand wurde wieder gestartet.":                                                                                                               "Activation failed. Your previous Mutti data was started again.",
	"Die App-Freigabe ist abgelaufen. Bitte Mutti erneut öffnen.":                                                                                                                                              "The app permission has expired. Please open Mutti again.",
	"Die Auswahl konnte nicht gespeichert werden.":                                                                                                                                                             "The selection could not be saved.",
	"Die Bibliothekszuordnung konnte nicht vollständig bestätigt werden: %s Der bisherige Datenstand bleibt aktiv.":                                                                                            "The library mapping could not be fully confirmed: %s Your previous data stays active.",
	"Die Datei hat sich während der Prüfung geändert.":                                                                                                                                                         "The file changed during the check.",
	"Die Geräteverbindung konnte nicht gestartet werden.":                                                                                                                                                      "The device connection could not be started.",
	"Die Intro-Skipper-Datenprüfung für %s ist fehlgeschlagen. Es wurde nicht umgeschaltet.":                                                                                                                   "Checking the Intro Skipper data for %s failed. Nothing was switched.",
	"Die Intro-Skipper-Datensicherung fehlt. Bei Fernimport bitte den aktuellen Mutti-Umzugshelfer verwenden.":                                                                                                 "The Intro Skipper data backup is missing. For a remote import, please use the current Mutti moving helper.",
	"Die Intro-Skipper-Prüfsumme stimmt nicht. Bitte das vollständige Mutti-Paket verwenden.":                                                                                                                  "The Intro Skipper checksum does not match. Please use the complete Mutti package.",
	"Die Intro-Skipper-Sicherung ist unvollständig.":                                                                                                                                                           "The Intro Skipper backup is incomplete.",
	"Die Jellyfin-Einstellung ist nicht eindeutig. Es wurde nichts verändert.":                                                                                                                                 "The Jellyfin setting is ambiguous. Nothing was changed.",
	"Die Jellyfin-Einstellungen haben sich geändert. Die Vorbereitung wurde angehalten.":                                                                                                                       "The Jellyfin settings have changed. The preparation was stopped.",
	"Die Jellyfin-Einstellungen haben sich während der Vorbereitung geändert. Es wurde nichts überschrieben.":                                                                                                  "The Jellyfin settings changed during the preparation. Nothing was overwritten.",
	"Die Jellyfin-Einstellungen wurden seit der Vorbereitung verändert. Der Import wurde angehalten; es wird nichts überschrieben.":                                                                            "The Jellyfin settings were changed since the preparation. The import was stopped; nothing will be overwritten.",
	"Die Jellyfin-Einstellungen wurden seit der Vorbereitung verändert. Es wird nichts überschrieben.":                                                                                                         "The Jellyfin settings were changed since the preparation. Nothing will be overwritten.",
	"Die Moduldaten der Sicherung konnten nicht vollständig übernommen werden; der vorherige Stand liegt in %s.":                                                                                               "The module data of the backup could not be restored completely; the previous state is in %s.",
	"Die Moduldaten der Sicherung sind unvollständig.":                                                                                                                                                         "The module data of the backup is incomplete.",
	"Die Ordner der Bibliothek %q stimmen nicht mit der vorgesehenen Übernahme überein.":                                                                                                                       "The folders of the library %q do not match the planned import.",
	"Die Plugin-Sicherung fehlt.":                                                                       "The plugin backup is missing.",
	"Die Plugin-Sicherung ist unvollständig.":                                                           "The plugin backup is incomplete.",
	"Die Prüfsumme der Sicherung stimmt nicht. Es wurde nichts umgeschaltet.":                           "The backup checksum does not match. Nothing was switched.",
	"Die Prüfung der Tabelle %s ist fehlgeschlagen: Anzahl stimmt nicht überein.":                       "Checking the table %s failed: the count does not match.",
	"Die Prüfung der Tabelle %s ist fehlgeschlagen: Inhalte stimmen nicht überein.":                     "Checking the table %s failed: the contents do not match.",
	"Die Prüfung der importierten Instanz hat zu lange gedauert.":                                       "Checking the imported instance took too long.",
	"Die Quelldateien haben sich während des Umzugs geändert. Bitte erneut versuchen.":                  "The source files changed during the move. Please try again.",
	"Die Quelle und Mutti müssen getrennte Datenordner verwenden.":                                      "The source and Mutti must use separate data folders.",
	"Die Quelleinstellungen wurden während des Imports verändert. Bitte erneut versuchen.":              "The source settings were changed during the import. Please try again.",
	"Die Sicherung enthält unsichere oder doppelte Dateipfade.":                                         "The backup contains unsafe or duplicate file paths.",
	"Die Sicherung enthält zu viele Dateien.":                                                           "The backup contains too many files.",
	"Die Sicherung ist unvollständig: %s fehlt.":                                                        "The backup is incomplete: %s is missing.",
	"Die Sicherung konnte nicht geöffnet werden.":                                                       "The backup could not be opened.",
	"Die Sicherung überschreitet die Größe dieses Teststands.":                                          "The backup exceeds the size supported by this test version.",
	"Die Tabelle %s fehlt nach der Wiederherstellung.":                                                  "The table %s is missing after the restore.",
	"Die Wiederherstellung ist unvollständig; der vorherige Stand liegt in %s.":                         "The restore is incomplete; the previous state is in %s.",
	"Die aktive Importinstanz ist ungültig.":                                                            "The active import instance is invalid.",
	"Die bisherigen Jellyfin-Einstellungen konnten nicht gesichert werden. Es wurde nichts verändert.":  "The previous Jellyfin settings could not be backed up. Nothing was changed.",
	"Die geprüfte Bibliothek wird aktiviert …":                                                          "Activating the verified library …",
	"Die importierte Einrichtung wurde nicht korrekt wiederhergestellt.":                                "The imported setup was not restored correctly.",
	"Die importierte Testinstanz konnte nicht gestartet werden. Der bisherige Datenstand bleibt aktiv.": "The imported test instance could not be started. Your previous data stays active.",
	"Die native Importfreigabe benötigt die lokale Mac-App.":                                            "The native import permission requires the local Mac app.",
	"Die ursprüngliche Kennung der Bibliothek %q fehlt.":                                                "The original identifier of the library %q is missing.",
	"Die Übernahme hat das Zeitlimit von 45 Minuten erreicht. Es wurde nicht umgeschaltet. Jellyfin kann die angeforderte Sicherung noch weiterführen; bitte vor einem neuen Versuch seinen Status prüfen.": "The import reached its 45-minute time limit. Nothing was switched. Jellyfin may still be finishing the requested backup; please check its status before trying again.",
	"Die übernommene Datei %s stimmt nicht mit der Sicherung überein.":                                                                                 "The imported file %s does not match the backup.",
	"Die übernommene Datenbank wird mit der Sicherung verglichen …":                                                                                    "Comparing the imported database with the backup …",
	"Diese Datenbank kann Mutti noch nicht automatisch vorbereiten. Es wurde nichts verändert.":                                                        "Mutti cannot prepare this database automatically yet. Nothing was changed.",
	"Diese Jellyfin-Datei gehört nicht ausschließlich zum angemeldeten Benutzer.":                                                                      "This Jellyfin file does not belong exclusively to the signed-in user.",
	"Diese Sicherung gehört zu %s; bitte mit genau dieser Version wiederherstellen.":                                                                   "This backup belongs to %s; please restore it with exactly that version.",
	"Diese Sicherung ist nicht mit dem geprüften Jellyfin-12.1-Import kompatibel.":                                                                     "This backup is not compatible with the verified Jellyfin 12.1 import.",
	"Diese Version (%s) ist älter als der Datenstand (%s). Ein älteres Programm kann migrierte Daten nicht sicher öffnen.":                             "This version (%s) is older than the data (%s). An older program cannot safely open migrated data.",
	"Dieser Teststand übernimmt Jellyfin 12.1. Für diese Serverversion ist die vollständige Übernahme noch nicht geprüft.":                             "This test version imports Jellyfin 12.1. A complete import is not yet verified for this server version.",
	"Dieses Mutti-Paket enthält keine Komponentenliste. Bitte Mutti neu installieren. Es wurde nichts verändert.":                                      "This Mutti package has no component list. Please reinstall Mutti. Nothing was changed.",
	"Dieses Mutti-Paket ist nicht mit dem Release-Schlüssel signiert. Bitte Mutti aus der offiziellen Quelle installieren. Es wurde nichts verändert.": "This Mutti package is not signed with the release key. Please install Mutti from the official source. Nothing was changed.",
	"Ein Benutzer verwendet einen externen Anmeldedienst. Dessen Übernahme muss zuerst eingerichtet werden.":                                           "A user signs in with an external authentication service. Taking it over must be set up first.",
	"Eine Konfigurationsdatei ist zu groß.":                                                                                                            "A configuration file is too large.",
	"Eine Wiederherstellung vor dem Update ist nur bei gesperrtem Start möglich.":                                                                      "Restoring the pre-update backup is only possible while the start is blocked.",
	"Eine neuere Version übernimmt das laufende Update; die Sicherung vor dem Update bleibt gültig.":                                                   "A newer version takes over the running update; the pre-update backup remains valid.",
	"Eine Übernahme läuft bereits.":                                                                                                               "An import is already running.",
	"Eine übernommene Bibliothek hat keine eindeutige Kennung.":                                                                                   "An imported library has no unique identifier.",
	"Eine übernommene Datei fehlt oder liegt außerhalb der Importinstanz.":                                                                        "An imported file is missing or lies outside the import instance.",
	"Empfangene Sicherung":                                                                                                                        "Received backup",
	"Entzogene Zugriffe konnten nicht übernommen werden; der vorherige Stand liegt in %s.":                                                        "Revoked access could not be carried over; the previous state is in %s.",
	"Für Intro Skipper bitte das vollständige Mutti-Paket verwenden.":                                                                             "For Intro Skipper, please use the complete Mutti package.",
	"Für die importierte Instanz fehlt der Prüfnachweis.":                                                                                         "The verification record for the imported instance is missing.",
	"Für die Übernahme ist ein aktiver Administratorzugang erforderlich.":                                                                         "The import needs an active administrator account.",
	"Für entfernte Server ist eine HTTPS-Adresse erforderlich. Lokal ist http://127.0.0.1 erlaubt.":                                               "Remote servers need an HTTPS address. Locally, http://127.0.0.1 is allowed.",
	"Intro Skipper %s benötigt noch eine geprüfte Datenmigration. Der bisherige Datenstand bleibt erhalten.":                                      "Intro Skipper %s still needs a verified data migration. Your existing data is kept.",
	"Intro Skipper einschließlich Einstellungen und vorhandener Analysedaten wurde geprüft übernommen.":                                           "Intro Skipper, including settings and existing analysis data, was verified and taken over.",
	"Intro Skipper ist enthalten und erkennt künftig Intros und Abspann.":                                                                         "Intro Skipper is included and will detect intros and credits from now on.",
	"Intro Skipper kann die mitgelieferte Audioanalyse noch nicht verwenden. Es wurde nicht umgeschaltet.":                                        "Intro Skipper cannot use the bundled audio analysis yet. Nothing was switched.",
	"Intro Skipper konnte nicht konsistent gesichert werden. Bitte Dateizugriff prüfen und laufende Analysen abwarten.":                           "Intro Skipper could not be backed up consistently. Please check file access and wait for running analyses to finish.",
	"Intro-Skipper-Daten wurden während des Umzugs geändert. Bitte laufende Analysen abwarten und erneut versuchen.":                              "Intro Skipper data changed during the move. Please wait for running analyses to finish and try again.",
	"Intro-Skipper-Einstellungen und Analysedaten werden gesichert …":                                                                             "Backing up Intro Skipper settings and analysis data …",
	"Jellyfin antwortet nicht auf die Anmeldung. Der Quellserver muss zuerst wieder erreichbar sein; es wurde keine weitere Sicherung gestartet.": "Jellyfin does not answer the sign-in. The source server must be reachable again first; no further backup was started.",
	"Jellyfin bleibt erhalten. Ab jetzt bitte Mutti verwenden; spätere Änderungen auf Jellyfin werden nicht synchronisiert.":                      "Jellyfin is kept. Please use Mutti from now on; later changes on Jellyfin are not synchronised.",
	"Jellyfin erstellt eine zweite Sicherung zur Abschlussprüfung …":                                                                              "Jellyfin is creating a second backup for the final check …",
	"Jellyfin hat die Vorbereitung nicht übernommen. Der Import wurde angehalten.":                                                                "Jellyfin did not apply the preparation. The import was stopped.",
	"Jellyfin hat die vorbereitete Einstellung nicht übernommen. Es wurde keine Sicherung gestartet.":                                             "Jellyfin did not apply the prepared setting. No backup was started.",
	"Jellyfin hat keine gültigen Datenbankeinstellungen geliefert.":                                                                               "Jellyfin did not return valid database settings.",
	"Jellyfin ist nach dem Neustart noch nicht bereit. Die bisherigen Einstellungen sind gesichert; bitte später erneut versuchen.":               "Jellyfin is not ready yet after the restart. The previous settings are backed up; please try again later.",
	"Jellyfin ist wieder bereit. Der Umzug wird fortgesetzt …":                                                                                    "Jellyfin is ready again. Continuing the move …",
	"Jellyfin konnte nicht automatisch neu gestartet werden. Die Vorbereitung bleibt für den nächsten Versuch gespeichert: %w":                    "Jellyfin could not be restarted automatically. The preparation is kept for the next attempt: %s",
	"Jellyfin konnte nicht automatisch vorbereitet werden. Die bisherigen Einstellungen sind gesichert: %w":                                       "Jellyfin could not be prepared automatically. The previous settings are backed up: %s",
	"Jellyfin sichert Benutzer, Bibliotheken und Metadaten …":                                                                                     "Jellyfin is backing up users, libraries and metadata …",
	"Jellyfin und Zugriffsrechte werden geprüft …":                                                                                                "Checking Jellyfin and access rights …",
	"Jellyfin verwendet den Datenbank-Sperrmodus Pessimistic. Dabei kann die Sicherung hängen bleiben. Bitte in Jellyfin den Standardmodus NoLock wählen und Jellyfin neu starten, bevor du den Umzug erneut beginnst. Mutti hat keine Sicherung gestartet und keine Quelldaten verändert.": "Jellyfin uses the database locking mode Pessimistic, which can make the backup hang. Please choose the default mode NoLock in Jellyfin and restart Jellyfin before you start the move again. Mutti has not started a backup and has not changed any source data.",
	"Jellyfin wird für den Umzug vorbereitet …":                     "Preparing Jellyfin for the move …",
	"Jellyfin wird neu gestartet. Mutti wartet auf deinen Server …": "Jellyfin is restarting. Mutti is waiting for your server …",
	"Jellyfin wurde während des Umzugs verändert. Bitte die Wiedergabe pausieren und die Übernahme erneut starten. Es wurde nicht umgeschaltet.":                                       "Jellyfin was changed during the move. Please pause playback and start the import again. Nothing was switched.",
	"Jellyfins Datenbankeinstellungen konnten vor der Sicherung nicht geprüft werden: %w":                                                                                              "Jellyfin's database settings could not be checked before the backup: %s",
	"Jellyfins Importvorbereitung konnte nicht geprüft werden: %w":                                                                                                                     "Jellyfin's import preparation could not be checked: %s",
	"Jellyfins Neustart konnte nicht bestätigt werden. Deine Daten bleiben erhalten; Mutti hat keine Sicherung gestartet. Beim nächsten Versuch wird die Vorbereitung erneut geprüft.": "Jellyfin's restart could not be confirmed. Your data is kept; Mutti has not started a backup. The preparation is checked again on the next attempt.",
	"Keine Jellyfin-Sicherung: manifest.json fehlt.":                                                                                                                                   "Not a Jellyfin backup: manifest.json is missing.",
	"Lokale Sicherung erstellt und Prüfsummen kontrolliert. Die Wiederherstellungsprobe steht noch aus.":                                                                               "Local backup created and checksums verified. The restore test is still pending.",
	"Medienoriginale bleiben an ihren bisherigen Speicherorten und müssen separat gesichert werden.":                                                                                   "Original media files stay where they are and must be backed up separately.",
	"Modul- oder Gerätedaten sind nicht mehr lesbar":                                                                                                                                   "module or device data can no longer be read",
	"Mutti wird bereits verwaltet.":                           "Mutti is already being managed.",
	"Mutti wird nach einem unerwarteten Ende neu gestartet …": "Mutti is restarting after an unexpected stop …",
	"Mutti wurde unerwartet beendet. Automatischer Wiederanlauf wartet; bei wiederholtem Fehler bitte das Paket und freien Speicher prüfen.": "Mutti stopped unexpectedly. An automatic restart is pending; if this keeps happening, please check the package and free storage.",
	"Mutti-Sicherung %s": "Mutti backup %s",
	"Nach dem Neustart antwortet ein anderer Server. Der Import wurde angehalten.":                              "A different server answers after the restart. The import was stopped.",
	"Nach dem Neustart antwortet ein anderer Server. Es wurde keine Sicherung gestartet.":                       "A different server answers after the restart. No backup was started.",
	"Netzwerkzugang und Transcoding sind auf Mutti angepasst. Hardwarebeschleunigung bei Bedarf erneut wählen.": "Network access and transcoding are adapted to Mutti. Choose hardware acceleration again if needed.",
	"Neuer Prüfversuch nach dem Update.":                                                                        "Checking again after the update.",
	"Nur die Mutti-App auf diesem Mac darf den Datenstand zurücksetzen.":                                        "Only the Mutti app on this Mac may roll back the data.",
	"Plugin-Programme werden nicht ungeprüft importiert.":                                                       "Plugin programs are not imported without verification.",
	"Programmpaket wird geprüft …":                                                                              "Checking the program package …",
	"Sicherung auf Jellyfin":                                                                                    "Backup on Jellyfin",
	"Sicherung ist unvollständig oder nicht mit diesem Paket kompatibel.":                                       "The backup is incomplete or not compatible with this package.",
	"Sicherung nicht gefunden.":                                                                                 "Backup not found.",
	"Sicherung und wiederhergestellte Daten wurden verglichen.":                                                 "The backup and the restored data were compared.",
	"Sicherung vor dem Update wird erstellt …":                                                                  "Creating the pre-update backup …",
	"Sicherung wiederhergestellt. Bitte erneut anmelden und Geräte neu koppeln.":                                "Backup restored. Please sign in again and pair devices again.",
	"Sicherung wird für Mutti vorbereitet …":                                                                    "Preparing the backup for Mutti …",
	"Unbekannte Datei in der Sicherung.":                                                                        "Unknown file in the backup.",
	"Unbekannte Daten in der Intro-Skipper-Sicherung.":                                                          "Unknown data in the Intro Skipper backup.",
	"Unbekannter Prüfpfad.":                                                                                     "Unknown verification path.",
	"Unerwartete Daten nach der Datenbanktabelle.":                                                              "Unexpected data after the database table.",
	"Ungültige Anfrage.":                                                                                        "Invalid request.",
	"Ungültige Datenbanktabelle im Archiv.":                                                                     "Invalid database table in the archive.",
	"Ungültige Intro-Skipper-Sicherung.":                                                                        "Invalid Intro Skipper backup.",
	"Ungültige Sicherung.":                                                                                      "Invalid backup.",
	"Ungültiger Prüfpfad.":                                                                                      "Invalid verification path.",
	"Ungültiger Serverpfad.":                                                                                    "Invalid server path.",
	"Ungültiger Sicherungsnachweis.":                                                                            "Invalid backup record.",
	"Ungültiger lokaler Kopplungszugang.":                                                                       "Invalid local pairing access.",
	"Ungültiger lokaler Modulzugang.":                                                                           "Invalid local module access.",
	"Ungültiger lokaler Ursprung.":                                                                              "Invalid local origin.",
	"Ungültiger lokaler Verwaltungszugang.":                                                                     "Invalid local management access.",
	"Update geprüft. Der Datenstand vor dem Update bleibt als Sicherung erhalten.":                              "Update verified. The pre-update data is kept as a backup.",
	"Vor dem Update konnte keine vollständige Sicherung erstellt werden: %s Es wurde nichts verändert.":         "No complete backup could be created before the update: %s Nothing was changed.",
	"Vorbereitete Sicherung":                                                                                    "Prepared backup",
	"Wiederherstellung geprüft, aber der Nachweis konnte nicht gespeichert werden.":                             "Restore verified, but the record could not be saved.",
	"Wiederherstellung in einer getrennten Instanz geprüft. Aktive Daten unverändert.":                          "Restore verified in a separate instance. Active data unchanged.",
	"Wiederherstellung in einer getrennten Testinstanz geprüft. Deine aktive Bibliothek bleibt unverändert.":    "Restore verified in a separate test instance. Your active library is unchanged.",
	"XML-Direktiven sind in einer Importdatei nicht erlaubt.":                                                   "XML directives are not allowed in an import file.",
	"der neue Server beendet sich wiederholt":                                                                   "the new server keeps stopping",
	"der neue Server ist nach 20 Minuten nicht bereit":                                                          "the new server is not ready after 20 minutes",
	"Übernahme abgebrochen. Deine bisherige Einrichtung bleibt erhalten. Eine bereits angeforderte Jellyfin-Sicherung kann auf der Quelle weiterlaufen.": "Import cancelled. Your existing setup is kept. A Jellyfin backup already requested may continue on the source.",
	"Übernahme abgebrochen. Der bisherige Datenstand bleibt erhalten.":                                                                                   "Import cancelled. Your existing data is kept.",
	"Übernahme abgebrochen. Es wurde nicht umgeschaltet.":                                                                                                "Import cancelled. Nothing was switched.",
}

type messagePattern struct {
	re      *regexp.Regexp
	verbs   []byte
	english string
}

var messagePatterns = compileMessagePatterns()

func compileMessagePatterns() []messagePattern {
	var out []messagePattern
	for german, english := range englishMessages {
		if !strings.Contains(german, "%") {
			continue
		}
		var expr strings.Builder
		var verbs []byte
		expr.WriteString("^")
		for i := 0; i < len(german); i++ {
			if german[i] == '%' && i+1 < len(german) {
				i++
				switch german[i] {
				case 's', 'v', 'w':
					expr.WriteString("(.+?)")
				case 'd':
					expr.WriteString(`(-?\d+)`)
				case 'q':
					expr.WriteString(`("(?:[^"\\]|\\.)*")`)
				default:
					panic("unsupported verb in message " + german)
				}
				verbs = append(verbs, german[i])
				continue
			}
			expr.WriteString(regexp.QuoteMeta(german[i : i+1]))
		}
		expr.WriteString("$")
		out = append(out, messagePattern{regexp.MustCompile(expr.String()), verbs, english})
	}
	// The most specific (longest) format wins when several match.
	sort.Slice(out, func(i, j int) bool { return len(out[i].re.String()) > len(out[j].re.String()) })
	return out
}

// translateMessage renders a manager message in English. Text that is not
// a known message (for example an error detail of another program) stays.
func translateMessage(text string) string {
	if english, ok := englishMessages[text]; ok {
		return english
	}
	for _, p := range messagePatterns {
		m := p.re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		args := make([]any, len(p.verbs))
		for i, verb := range p.verbs {
			switch verb {
			case 'd':
				n, _ := strconv.Atoi(m[i+1])
				args[i] = n
			case 'q':
				s, err := strconv.Unquote(m[i+1])
				if err != nil {
					s = m[i+1]
				}
				args[i] = s
			default:
				args[i] = translateMessage(m[i+1])
			}
		}
		return fmt.Sprintf(p.english, args...)
	}
	return text
}

// say renders a manager message in the language of the request.
func say(r *http.Request, text string) string {
	if englishRequest(r) {
		return translateMessage(text)
	}
	return text
}

// englishRequest: an explicit ?language= wins, then the first supported
// Accept-Language entry. Without either the manager answers in German, as
// before; the Mac app and browsers always send the user's languages.
func englishRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if l := r.URL.Query().Get("language"); l != "" {
		return strings.HasPrefix(strings.ToLower(l), "en")
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		tag = strings.ToLower(tag)
		switch {
		case strings.HasPrefix(tag, "en"):
			return true
		case strings.HasPrefix(tag, "de"):
			return false
		}
	}
	return false
}
