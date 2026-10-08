// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import "net/http"

// englishMessages translates the hub's user-facing texts. The German text is
// the message ID (like gettext); codes stay the stable contract for clients.
// TestEveryUserMessageHasAnEnglishText keeps this table complete.
var englishMessages = map[string]string{
	"Anmeldung abgelaufen oder Zugriff entfernt.":                                                       "Sign-in expired or access removed.",
	"Anmeldung bei der Fotobibliothek fehlgeschlagen. Bitte E-Mail und Passwort prüfen.":                "Signing in to the photo library failed. Please check email and password.",
	"Anmeldung beim Dokumentenarchiv fehlgeschlagen. Bitte Benutzername und Passwort prüfen.":           "Signing in to the document archive failed. Please check user name and password.",
	"Auf dem Server ist nicht genug Speicher frei.":                                                     "There is not enough free storage on the server.",
	"Bitte das Modell zuerst laden.":                                                                    "Please download the model first.",
	"Bitte die Funktion zuerst einrichten.":                                                             "Please set up this feature first.",
	"Bitte ein aktives Wiedergabeprofil ohne Administratorrechte wählen.":                               "Please choose an active playback profile without administrator rights.",
	"Bitte ein normales Immich-Konto verwenden, nicht das Administratorkonto.":                          "Please use a regular Immich account, not the administrator account.",
	"Bitte ein normales Paperless-Konto verwenden, kein Superuser-Konto.":                               "Please use a regular Paperless account, not a superuser account.",
	"Bitte eine Adresse wie http://192.168.1.10:2283 eingeben.":                                         "Please enter an address such as http://192.168.1.10:2283.",
	"Bitte eine Nachricht mit höchstens 4000 Zeichen senden.":                                           "Please send a message of at most 4000 characters.",
	"Bitte nur die Serveradresse ohne weiteren Pfad eingeben.":                                          "Please enter only the server address without a path.",
	"Bitte warte, bis die laufende Antwort fertig ist, oder brich sie ab.":                              "Please wait until the current answer is finished, or cancel it.",
	"Bitte zuerst die Dienstadresse festlegen.":                                                         "Please set the service address first.",
	"Das Dokumentenarchiv ist gerade nicht erreichbar.":                                                 "The document archive is currently unreachable.",
	"Das Dokumentenarchiv ist gerade nicht erreichbar. Filme und Serien bleiben nutzbar.":               "The document archive is currently unreachable. Movies and shows remain available.",
	"Das gewählte Modell ist nicht installiert oder nicht die geprüfte Version.":                        "The selected model is not installed or not the verified version.",
	"Der Dienst hat den Zugriff für dieses Profil abgelehnt. Bitte die Kontozuordnung in Mutti prüfen.": "The service denied access for this profile. Please check the account assignment in Mutti.",
	"Der Dienst ist gerade nicht erreichbar.":                                                           "The service is currently unreachable.",
	"Der Dienstname ist im Heimnetz nicht auflösbar.":                                                   "The service name cannot be resolved in the home network.",
	"Der Download konnte nicht gestartet werden.":                                                       "The download could not be started.",
	"Der Import ist fehlgeschlagen.":                                                                    "The import failed.",
	"Der Upload wurde unterbrochen. Bitte erneut versuchen.":                                            "The upload was interrupted. Please try again.",
	"Der Upload wurde unterbrochen. Bitte erneut versuchen; doppelte Fotos werden erkannt.":             "The upload was interrupted. Please try again; duplicate photos are detected.",
	"Die Aktion konnte nicht sicher vorgemerkt werden. Es wurde nichts geändert.":                       "The action could not be recorded safely. Nothing was changed.",
	"Die Antwort hat zu lange gedauert und wurde beendet.":                                              "The answer took too long and was stopped.",
	"Die Datei ist größer als 20 MB.":                                                                   "The file is larger than 20 MB.",
	"Die Datei ist zu groß.":                                                                            "The file is too large.",
	"Die Fotobibliothek hat keinen Zugriffsschlüssel ausgestellt.":                                      "The photo library did not issue an access key.",
	"Die Fotobibliothek ist gerade nicht erreichbar. Filme und Serien bleiben nutzbar.":                 "The photo library is currently unreachable. Movies and shows remain available.",
	"Die Mediathek ist gerade nicht erreichbar.":                                                        "The media library is currently unreachable.",
	"Die Suche ist abgelaufen oder die Freigaben haben sich geändert. Bitte neu suchen.":                "The search expired or permissions changed. Please search again.",
	"Die lokale KI hat die Anfrage nicht beantwortet.":                                                  "The local AI did not answer the request.",
	"Die lokale KI ist gerade nicht bereit. Suche und Wiedergabe bleiben nutzbar.":                      "The local AI is not ready right now. Search and playback remain available.",
	"Die lokale KI startet gerade oder ist nicht bereit.":                                               "The local AI is starting or not ready.",
	"Die lokale KI-Engine antwortet nicht.":                                                             "The local AI engine does not respond.",
	"Die lokale KI-Engine konnte nicht gestartet werden.":                                               "The local AI engine could not be started.",
	"Die vorhandene Modellversion ist nicht die geprüfte Version.":                                      "The existing model version is not the verified version.",
	"Die Übernahme ist nur aus einer lokalen Mac-Installation möglich.":                                 "Adopting is only possible from a local Mac installation.",
	"Diese Funktion ist auf diesem Mutti noch nicht eingerichtet.":                                      "This feature is not set up on this Mutti yet.",
	"Diese Funktion ist derzeit ausgeschaltet.":                                                         "This feature is currently switched off.",
	"Diese KI-Aufgabe ist für die gewählte Konfiguration noch nicht freigegeben.":                       "This AI task is not yet released for the current configuration.",
	"Dieses Dateiformat kann das Dokumentenarchiv nicht übernehmen.":                                    "The document archive cannot accept this file format.",
	"Dieses Gespräch hat bereits 20 Anhänge.":                                                           "This conversation already has 20 attachments.",
	"Dieses Konto ist bereits einem anderen Profil zugeordnet. Jedes Profil braucht ein eigenes Konto.": "This account is already assigned to another profile. Each profile needs its own account.",
	"Dieses Modell gehört nicht zum geprüften Katalog.":                                                 "This model is not part of the verified catalogue.",
	"Dieses Modell liegt in der angegebenen lokalen Installation nicht vor.":                            "This model is not present in the given local installation.",
	"Dieses Paket enthält keine lokale KI-Engine.":                                                      "This package contains no local AI engine.",
	"Es laufen bereits Antworten für dieses Profil. Bitte kurz warten.":                                 "Answers are already running for this profile. Please wait a moment.",
	"Es läuft bereits ein Download.":                                                                    "A download is already running.",
	"Für den Upload fehlen Dateikennung oder Aufnahmedatum.":                                            "The upload is missing the file ID or capture date.",
	"Für dieses Modell hat der Server zu wenig Arbeitsspeicher.":                                        "The server has too little memory for this model.",
	"Für dieses Profil ist noch kein Konto zugeordnet.":                                                 "No account is assigned to this profile yet.",
	"Für dieses Profil nicht freigegeben.":                                                              "Not allowed for this profile.",
	"Nicht gefunden oder nicht freigegeben.":                                                            "Not found or not shared.",
	"Nur Dienste auf diesem Gerät oder im privaten Heimnetz sind erlaubt.":                              "Only services on this device or in the private home network are allowed.",
	"Unerwartete Antwort des Dienstes.":                                                                 "Unexpected response from the service.",
	"Unerwartete Antwort des Dokumentenarchivs.":                                                        "Unexpected response from the document archive.",
	"Ungültige Anfrage.": "Invalid request.",
	"Unter diesem Pfad liegt kein lokaler Modellspeicher.":   "There is no local model store at this path.",
	"Unter dieser Adresse antwortet kein passender Dienst.":  "No matching service answers at this address.",
	"Unter dieser Adresse antwortet keine lokale KI-Engine.": "No local AI engine answers at this address.",
	"Unterstützt werden Text- und PDF-Dateien bis 20 MB.":    "Text and PDF files up to 20 MB are supported.",
	"Diese Antwortsprache wird nicht unterstützt.":           "This answer language is not supported.",
	"Neues Gespräch": "New conversation",
	"Die Antwort wurde durch einen Neustart unterbrochen.":  "The answer was interrupted by a restart.",
	"Die Antwort wurde wegen der Längenbegrenzung gekürzt.": "The answer was shortened because of the length limit.",
	"Die Antwort konnte nicht erstellt werden.":             "The answer could not be created.",
	"Abgebrochen.":                                                                                 "Cancelled.",
	"%d ungültige Quellenmarke(n) entfernt":                                                        "%d invalid source marker(s) removed",
	"Der Film ist für dieses Profil nicht mehr verfügbar.":                                         "The movie is no longer available for this profile.",
	"Die Änderung wurde nicht übernommen.":                                                         "The change was not applied.",
	"Ergebnis unklar. Bitte den Film in der Bibliothek prüfen.":                                    "Result unclear. Please check the movie in the library.",
	"Mutti wurde während der Ausführung beendet. Bitte das Ergebnis in der Bibliothek prüfen.":     "Mutti stopped while the action ran. Please check the result in the library.",
	"Dieses Dokument ist bereits im Archiv.":                                                       "This document is already in the archive.",
	"Die lokale KI-Engine wurde unerwartet beendet.":                                               "The local AI engine stopped unexpectedly.",
	"Der Download ist fehlgeschlagen. Bitte Internetverbindung und freien Speicher prüfen.":        "The download failed. Please check the internet connection and free storage.",
	"Das geladene Modell stimmt nicht mit der geprüften Version überein und wird nicht verwendet.": "The downloaded model does not match the verified version and is not used.",
	"Die Übernahme ist fehlgeschlagen; das Modell wird nicht verwendet.":                           "Adopting failed; the model is not used.",
}

// say renders a hub message in the pack's language; German is the source.
func (l *languagePack) say(text string) string {
	if l != nil && l.Code == "en" {
		if t, ok := englishMessages[text]; ok {
			return t
		}
	}
	return text
}

// messageLanguage of a request: explicit ?language=, else Accept-Language,
// else the default. An unsupported explicit value falls back too.
func responseLanguage(r *http.Request) *languagePack {
	if r == nil {
		return languagePacks[defaultLanguage]
	}
	if l, err := requestLanguage(r.URL.Query().Get("language"), r); err == nil {
		return l
	}
	l, _ := requestLanguage("", r)
	return l
}
