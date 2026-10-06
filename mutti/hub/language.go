// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
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
	// A favourite change request needs both a favourite word and a change
	// verb in the imperative or infinitive (not a participle as in "is it
	// marked as favourite?"); see requestsFavorite.
	favoriteWord *regexp.Regexp
	changeVerb   *regexp.Regexp
	// politeAsk turns a question into a request ("can you …", "bitte").
	politeAsk *regexp.Regexp
	// interrogative: a sentence that starts like a question.
	interrogative *regexp.Regexp
	negation      *regexp.Regexp
	// reported marks a sentence that relays a document's content.
	reported *regexp.Regexp
	// The draft reports that the results do not answer the question.
	nothingFound *regexp.Regexp
	// The question is about the user's own things, not general knowledge.
	ownData *regexp.Regexp
	// Claims that a change already happened (rule 7), see claimsChange:
	// a favourite change, a first-person deletion, or a passive deletion at
	// the end of a clause (not a reported document fact).
	claimFavorite *regexp.Regexp
	claimSelf     *regexp.Regexp
	claimPassive  *regexp.Regexp
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
		favoriteWord:    regexp.MustCompile(`(?i)favorit`),
		changeVerb:      regexp.MustCompile(`(?i)\b(markiere?n?|setze?n?|füge?n?|hinzufügen|hinzu|aufnehmen|nimm|nehme?n?|packe?n?|mache?n?|entferne?n?|streiche?n?|lösche?n?|speichere?n?|lege?n?|soll|sollen)\b`),
		politeAsk:       regexp.MustCompile(`(?i)\b(kannst|könntest|würdest|magst|willst|kann|könnte|würde)\s+(du|sie)\b|\bbitte\b`),
		interrogative:   regexp.MustCompile(`(?i)^[\s"„“»]*(wer|wen|wem|wessen|was|wann|wo|woher|wohin|womit|wozu|worüber|worum|wie|welche[mnrs]?|warum|weshalb|wieso|wieviel|habe|hab|hast|hat|haben|gibt|ist|sind|war|waren|kann|kannst|darf|muss|soll|sollte)\b`),
		negation:        regexp.MustCompile(`(?i)\b(nicht|nichts|kein|keine|keinen|keinem|keiner|keines|nie|niemals)\b`),
		reported:        regexp.MustCompile(`(?i)\b(laut|gemäß|zufolge|steht)\b`),
		nothingFound:    regexp.MustCompile(`(?i)(nicht gefunden|nichts (gefunden|passendes)|\bkeine?[nrs]?\b [^.,;]{0,60}(gefunden|vorhanden)|gibt es [^.]{0,40}(nicht|kein))`),
		ownData:         regexp.MustCompile(`(?i)\b(mein|meine|meinem|meinen|meiner|meines|mir|ich|wir|uns|unser|unsere|unserem|unseren|unserer|unseres)\b`),
		claimFavorite:   regexp.MustCompile(`(?i)\b(wurde|wurden|habe|hab|ist jetzt|sind jetzt|ist nun|sind nun|ist ab sofort|ist bereits|sind bereits)\b[^.]{0,60}(als favorit(en)? (markiert|gesetzt|gespeichert)|(zu|in|auf) (den |deinen |ihren |die |deine |ihre )?favoriten(liste)? (hinzugefügt|aufgenommen|gespeichert|gesetzt)|aus (den |deinen |ihren |der |deiner |ihrer )?favoriten(liste)? (entfernt|genommen|gelöscht|gestrichen))|favoriten (wurde|wurden)\b[^.]{0,30}(entfernt|gelöscht)`),
		claimSelf:       regexp.MustCompile(`(?i)\b(ich habe|habe ich|hab ich|ich hab)\b[^.]{0,60}\b(gelöscht|entfernt)\s*(\[Q\d+\]\s*)?([,).!?;\n]|$|und\b|oder\b|aber\b)`),
		claimPassive:    regexp.MustCompile(`(?i)\b(wurde|wurden|ist jetzt|sind jetzt|ist nun|sind nun|ist bereits|sind bereits)\b[^.]{0,60}\b(gelöscht|entfernt)\s*(\[Q\d+\]\s*)?([,).!?;\n]|$|und\b|oder\b|aber\b)`),
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
		favoriteWord:    regexp.MustCompile(`(?i)favou?rite`),
		changeVerb:      regexp.MustCompile(`(?i)\b(mark|unmark|add|put|remove|unfavou?rite|make|set|take|drop|save|delete)\b`),
		politeAsk:       regexp.MustCompile(`(?i)\b(can|could|would|will) you\b|\bplease\b`),
		interrogative:   regexp.MustCompile(`(?i)^[\s"“]*(who|whom|whose|what|when|where|which|why|how|do|does|did|is|are|was|were|have|has|had|can|could|should|would|will|am)\b`),
		negation:        regexp.MustCompile(`(?i)\b(not|no|nothing|never|none|neither|nor|cannot)\b|n't\b`),
		reported:        regexp.MustCompile(`(?i)\b(according to|states that|says that|it says)\b`),
		nothingFound:    regexp.MustCompile(`(?i)(not found|nothing (found|matching|relevant)|\bno\b [^.,;]{0,40}(found|available)|could(n't| not) find|there (is|are) no\b)`),
		ownData:         regexp.MustCompile(`(?i)\b(my|mine|me|i|i'm|i've|we|us|our|ours)\b`),
		claimFavorite:   regexp.MustCompile(`(?i)\b(has been|have been|is now|are now|was|were|i've|i have)\b[^.]{0,60}\b(marked [^.]{0,40}?as (a |your )?favou?rite|added [^.]{0,40}?to (your |the |my )?favou?rites|removed [^.]{0,40}?from (your |the |my )?favou?rites|(marked|added|removed) as (a |your )?favou?rite)|\bi (just |already )?(marked|added|removed)\b[^.]{0,60}favou?rite`),
		claimSelf:       regexp.MustCompile(`(?i)\b(i've|i have|i)( just| already)? (deleted|removed)\b`),
		claimPassive:    regexp.MustCompile(`(?i)\b(has been|have been|was|were|is now|are now)\b[^.]{0,60}\b(deleted|removed)( from [^,;]{0,40})?\s*(\[Q\d+\]\s*)?([,).!?;\n]|$|and\b|but\b)`),
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

var (
	// sentenceSpan keeps each sentence with its end mark, so "?" stays visible.
	sentenceSpan = regexp.MustCompile(`[^.!?\n;]+[.!?\n;]*`)
	// quoted text (titles, quoted messages) is not the answer's own wording.
	quoted = regexp.MustCompile(`„[^“”"\n]{0,200}[“”"]|“[^”"\n]{0,200}[”"]|"[^"\n]{0,200}"|«[^»\n]{0,200}»|»[^«\n]{0,200}«`)
)

func sentences(text string) []string { return sentenceSpan.FindAllString(text, -1) }

// isQuestion: a sentence with a question mark or a typical question start.
// Offers to search only matter for information questions, not for requests
// like "Lösch bitte alle meine Fotos."
func (l *languagePack) isQuestion(text string) bool {
	if strings.Contains(text, "?") {
		return true
	}
	for _, s := range sentences(text) {
		if l.interrogative.MatchString(s) {
			return true
		}
	}
	return false
}

// requestsFavorite: some sentence asks to change a favourite, either as an
// instruction or as a polite question ("Kannst du … markieren?"), but not a
// question about the current state ("Ist Nordlicht als Favorit markiert?").
func (l *languagePack) requestsFavorite(text string) bool {
	for _, s := range sentences(text) {
		if !l.favoriteWord.MatchString(s) || !l.changeVerb.MatchString(s) {
			continue
		}
		if l.isQuestion(s) && !l.politeAsk.MatchString(s) {
			continue
		}
		return true
	}
	return false
}

// claimsChange reports a sentence that says a change already happened.
// Negated clauses ("Es wurde nichts gelöscht.") are honest; passive
// deletions that relay a cited or reported document fact ("Laut Rechnung
// wurde ein Zahn entfernt [Q1].") are not claims about Mutti's actions.
func (l *languagePack) claimsChange(text string) bool {
	for _, s := range sentences(text) {
		for _, re := range []*regexp.Regexp{l.claimFavorite, l.claimSelf, l.claimPassive} {
			for _, loc := range re.FindAllStringIndex(s, -1) {
				// The clause of the match: a negation after a comma belongs to
				// another statement ("…markiert, du musst nichts tun").
				start := 0
				if i := strings.LastIndexAny(s[:loc[0]], ",:–—("); i >= 0 {
					_, size := utf8.DecodeRuneInString(s[i:])
					start = i + size
				}
				if l.negation.MatchString(s[start:loc[1]]) {
					continue
				}
				if re == l.claimPassive && (hasMarker(s) || l.reported.MatchString(s)) {
					continue
				}
				return true
			}
		}
	}
	return false
}

// matchesOutside reports a match of re in text that is not just an echo of
// the user's own words or a quotation, e.g. explaining the router message
// "Datei nicht gefunden" is not a claim about the user's files.
func matchesOutside(re *regexp.Regexp, text, question string) bool {
	for _, m := range re.FindAllString(quoted.ReplaceAllString(text, " "), -1) {
		if !strings.Contains(strings.ToLower(question), strings.ToLower(m)) {
			return true
		}
	}
	return false
}

// wrongLanguage reports a text dominated by another supported language's
// function words. Quoted text and source titles (movie, document or photo
// names in their original language) do not count.
func (l *languagePack) wrongLanguage(text string, titles ...string) bool {
	for _, title := range titles {
		if len(title) >= 3 {
			text = strings.ReplaceAll(text, title, " ")
		}
	}
	text = markerPattern.ReplaceAllString(quoted.ReplaceAllString(text, " "), " ")
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
