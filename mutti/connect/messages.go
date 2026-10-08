// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"net/http"
	"strings"
)

// englishMessages translates the texts the pairing page, devices and the
// signalling service receive. German is the message ID; a response is
// translated when the request asks for English (Accept-Language).
var englishMessages = map[string]string{
	"Bitte mit dem lokalen Besitzerzugang anmelden.":                                                    "Please sign in with the local owner account.",
	"Besitzeranmeldung erforderlich.":                                                                   "Owner sign-in required.",
	"Profile konnten nicht geladen werden.":                                                             "Profiles could not be loaded.",
	"Sperren fehlgeschlagen.":                                                                           "Blocking failed.",
	"Profil konnte nicht erstellt werden.":                                                              "The profile could not be created.",
	"Anfrage abgelaufen. Bitte neu koppeln.":                                                            "Request expired. Please pair again.",
	"Bitte ein aktives Wiedergabeprofil ohne Administratorrechte wählen.":                               "Please choose an active playback profile without administrator rights.",
	"Quick Connect muss in Mutti aktiviert sein, damit das freigegebene Profil angemeldet werden kann.": "Quick Connect must be enabled in Mutti so the approved profile can sign in.",
	"Profilanmeldung fehlgeschlagen.":                                                                   "Profile sign-in failed.",
	"Anfrage inzwischen abgelaufen.":                                                                    "The request has expired in the meantime.",
	"Mutti ist offline.":                                                                                "Mutti is offline.",
	"Mutti antwortet nicht.":                                                                            "Mutti does not answer.",
	"Der Vermittlungsdienst ist nicht erreichbar.":                                                      "The signalling service is unreachable.",
	"Mutti ist nicht erreichbar oder der Verbindungsdienst ist ausgelastet.":                            "Mutti is unreachable or the connection service is busy.",
	"Bitte bestehende QR-Codes ablaufen lassen.":                                                        "Please let the existing QR codes expire first.",
	"Gerät ist nicht freigegeben oder wurde gesperrt.":                                                  "This device is not approved or was blocked.",
	"Wiedergabeprofil nicht verfügbar.":                                                                 "Playback profile not available.",
	"Mutti-Mediendienst nicht erreichbar.":                                                              "The Mutti media service is unreachable.",
	"Diese Mutti-Version bietet keine Zusatzfunktionen an.":                                             "This Mutti version offers no additional features.",
	"Die Zusatzfunktionen von Mutti sind gerade nicht erreichbar. Filme und Serien bleiben nutzbar.":    "Mutti's additional features are unreachable right now. Movies and shows remain available.",
	"QR-Code abgelaufen oder bereits verwendet.":                                                        "QR code expired or already used.",
	"Jellyfin hat die Anfrage abgelehnt.":                                                               "Jellyfin rejected the request.",
}

// say returns text in the language of the request; unknown text stays.
func say(r *http.Request, text string) string {
	if r == nil {
		return text
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		tag = strings.ToLower(tag)
		if strings.HasPrefix(tag, "de") {
			return text
		}
		if strings.HasPrefix(tag, "en") {
			if english, ok := englishMessages[text]; ok {
				return english
			}
			return text
		}
	}
	return text
}
