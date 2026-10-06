// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

// languagePack holds everything that depends on the user's language. The
// model-facing contract (system prompt, tool schemas and results, server
// notes) is canonical English; a pack selects the answer language,
// recognises user requests and model drafts in that language and renders the
// few fixed texts the user sees. Qualification is bound to one pack.
type languagePack struct {
	Code string // BCP 47 primary language subtag
	Name string // English name used in the prompt
	// Example search terms in this language for rule 3.
	searchExample string
	// A user request that only an action tool can fulfil.
	favoriteRequest *regexp.Regexp
	// The draft reports that the results do not answer the question.
	nothingFound *regexp.Regexp
	// The question is about the user's own things, not general knowledge.
	ownData *regexp.Regexp
	// Claims that a change already happened (rule 7); also used by scorers.
	claimed *regexp.Regexp
	// Offering to search or asking the user for their documents instead of
	// looking them up; only counts for questions about the user's own data.
	deflection *regexp.Regexp
	// Statements that only a tool result could justify.
	unverifiedClaim *regexp.Regexp
	// The draft offers to search again or asks for other terms instead of
	// searching once more itself.
	offerRetry     *regexp.Regexp
	stop           map[string]bool
	functionWords  map[string]bool
	proposalNotice string
	found          map[string]string // tool status shown to the user
}

var languagePacks = map[string]*languagePack{
	"de": {
		Code: "de", Name: "German", searchExample: "Handyvertrag Laufzeit",
		favoriteRequest: regexp.MustCompile(`(?i)(als favorit|(zu|zum|zur) (den |dem |meinen |meinem |meiner )?favorit|aus (den |meinen )?favoriten|favorit (markier|setz|entfern)|favoriten(liste)? (hinzu|aufnehm|entfern))`),
		nothingFound:    regexp.MustCompile(`(?i)(nicht gefunden|nichts (gefunden|passendes)|keine?[nrs]? [^.]{0,60}(gefunden|vorhanden)|gibt es [^.]{0,40}(nicht|kein))`),
		ownData:         regexp.MustCompile(`(?i)\b(mein|meine|meinem|meinen|meiner|meines|mir|ich)\b`),
		claimed:         regexp.MustCompile(`(?i)(wurde|habe|ist jetzt|sind jetzt)[^.]{0,40}(als favorit markiert|gelöscht|entfernt\b)`),
		deflection:      regexp.MustCompile(`(?i)(suche ich (gerne|gern)|soll ich [^.?]{0,30}suchen|dass ich [^.?]{0,30}suche|(hast|haben) (du|sie) [^.?]{0,40}(dokument|rechnung|unterlagen|archiv|vertrag)|(gib|schick|nenne|zeig) mir [^.?]{0,30}(text|suchbegriff|auszug|dokument|vertrag|unterlagen))`),
		unverifiedClaim: regexp.MustCompile(`(?i)(nicht gefunden|nichts gefunden|keine (passenden |relevanten )?(informationen|dokumente|treffer|filme|fotos|einträge)|habe[^.]{0,80}(gesucht|nachgesehen|durchsucht|gefunden)|keinen zugriff auf (deine|ihre|dein|ihr) (persönlichen|privaten|dokumente|daten|unterlagen|rechnungen|steuer))`),
		offerRetry:      regexp.MustCompile(`(?i)(andere[nmr]? (such)?begriff|(noch ?(ein)?mal|erneut|weiter) (such|nachseh|schau)|möchtest du,? dass ich|soll ich [^.?]{0,40}(such|schau))`),
		stop:            wordSet(`aber alle allem auch auf aus bei bin bis bitte da dann das dass dem den denn der des die dies diese diesem dieser doch du ein eine einem einen einer eines es etwas für gibt hab habe haben hat hatte hier hoch ich ihm ihn ihr ihre im in ist ja kann kannst laut lautet mal man mein meine meinem meinen meiner meines mich mir mit muss nach nenne nicht noch nur ob oder pro sag sage schon sein seine sich sie sind so steht stehen über um und uns unter viel viele vom von war waren was wann warum welche welchem welchen welcher welches wer wie wieviel wird wo zeig zeige zu zum zur`),
		functionWords:   wordSet(`der die das ist sind und nicht ein eine ich du sie es mit für auf zu den dem im noch wurde habe hat keine kein bitte auch`),
		proposalNotice:  "Ich habe einen Vorschlag angelegt. Bitte bestätige ihn; vorher wird nichts geändert.",
		found: map[string]string{"movies": "%d Filme gefunden", "documents": "%d Dokumente gefunden", "photos": "%d Fotos/Videos gefunden",
			"proposal": "Vorschlag wartet auf Bestätigung"},
	},
	"en": {
		Code: "en", Name: "English", searchExample: "phone contract term",
		favoriteRequest: regexp.MustCompile(`(?i)(as (a |my )?favou?rite|to (my |the )?favou?rites|from (my |the )?favou?rites|(mark|add|remove|unmark|unfavou?rite|make|set)[^.?!]{0,40}favou?rite)`),
		nothingFound:    regexp.MustCompile(`(?i)(not found|nothing (found|matching|relevant)|no [^.]{0,40}(found|available)|could(n't| not) find|there (is|are) no)`),
		ownData:         regexp.MustCompile(`(?i)\b(my|mine|me|i|i'm|i've)\b`),
		claimed:         regexp.MustCompile(`(?i)(has been|have been|is now|are now|i've|i have)[^.]{0,40}(marked as (a )?favou?rite|added to (your )?favou?rites|deleted|removed\b)`),
		deflection:      regexp.MustCompile(`(?i)(happy to search|shall i search|should i search|want me to search|(do|can|could) you (have|share|send|give|provide)[^.?]{0,40}(document|invoice|bill|contract|text|archive)|(send|give|show|tell) me [^.?]{0,30}(text|search term|excerpt|document|contract))`),
		unverifiedClaim: regexp.MustCompile(`(?i)(not found|nothing found|no (matching |relevant )?(information|documents|results|movies|photos|entries)|i (have |'ve )?(searched|looked|checked)|(didn't|did not|couldn't|could not) find|(don't|do not) have access to your (personal|private|documents|data|files|invoices|tax))`),
		offerRetry:      regexp.MustCompile(`(?i)(other (search )?(terms|keywords)|search again|try (again|another|different)|(would|do) you (like|want) me to (search|look|try)|shall i (search|look|try))`),
		stop:            wordSet(`a about all also am an and any are as at be been but by can could did do does for from get give has have how i if in is it its many me much my of on or please show should tell than that the their there these this to was what when where which who why will with would you your`),
		functionWords:   wordSet(`the is are was were has have been and of to it this that not you your with for cannot can't i it's does do will would should`),
		proposalNotice:  "I've prepared a suggestion. Please confirm it; nothing changes before that.",
		found: map[string]string{"movies": "%d movies found", "documents": "%d documents found", "photos": "%d photos/videos found",
			"proposal": "Suggestion awaiting confirmation"},
	},
}

// defaultLanguage answers clients that do not state a language. English is
// the canonical product language; German is one supported option.
const defaultLanguage = "en"

func languageFor(code string) (*languagePack, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	if i := strings.IndexAny(code, "-_"); i > 0 {
		code = code[:i]
	}
	l, ok := languagePacks[code]
	return l, ok
}

// requestLanguage takes an explicit choice, else the first supported
// Accept-Language entry, else the default. Language selects behaviour and the
// matching qualification; it grants nothing.
func requestLanguage(explicit string, r *http.Request) (*languagePack, error) {
	if explicit != "" {
		if l, ok := languageFor(explicit); ok {
			return l, nil
		}
		return nil, apiErr(400, "unsupported_language", "Diese Antwortsprache wird nicht unterstützt.")
	}
	if r != nil {
		for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
			tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
			if l, ok := languageFor(tag); ok {
				return l, nil
			}
		}
	}
	return languagePacks[defaultLanguage], nil
}

func wordSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// wrongLanguage reports a text dominated by another supported language's
// function words.
func (l *languagePack) wrongLanguage(text string) bool {
	own := 0
	other := map[string]int{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
		if l.functionWords[w] {
			own++
		}
		for code, p := range languagePacks {
			if code != l.Code && p.functionWords[w] {
				other[code]++
			}
		}
	}
	for _, n := range other {
		if n >= 3 && n > 2*own {
			return true
		}
	}
	return false
}

// searchTerms turns a question into an OR query for the document archive.
func (l *languagePack) searchTerms(question string) string {
	terms := []string{}
	seen := map[string]bool{}
	for _, w := range searchWord.FindAllString(attachmentRef.ReplaceAllString(question, ""), -1) {
		lw := strings.ToLower(w)
		if l.stop[lw] || seen[lw] {
			continue
		}
		seen[lw] = true
		terms = append(terms, w)
		if len(terms) == 6 {
			break
		}
	}
	return strings.Join(terms, " OR ")
}
