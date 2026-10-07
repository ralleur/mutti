// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Source is a server-registered reference. The model only sees Ref ("Q3");
// ObjectID never leaves the server towards the model.
type Source struct {
	Ref      string         `json:"ref"`
	Service  string         `json:"service"` // media, documents, photos, attachment
	Kind     string         `json:"kind"`    // movie, document, image, video, attachment
	ObjectID string         `json:"id"`
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle,omitempty"`
	Facts    map[string]any `json:"-"`
	// SourceRef provenance (contracts.md): stable content reference with the
	// revision seen when the source was retrieved, and what part was used.
	Content *ContentRef `json:"content,omitempty"`
	// Instance is the backend instance the object belongs to; the same
	// object number in a replaced backend is a different object.
	Instance  string         `json:"instance,omitempty"`
	Locator   *SourceLocator `json:"locator,omitempty"`
	Retrieved *time.Time     `json:"retrieved,omitempty"`
	Revision  string         `json:"-"` // backend revision while a run is active
}

// sourceBook allocates conversation-scoped markers. A run works on a copy and
// the result is merged when the run is stored.
type sourceBook struct {
	Items map[string]*Source `json:"items"`
	Next  int                `json:"next"`
	// touched lists the markers a run used, cited or not.
	touched map[string]bool
}

func (b *sourceBook) touch(ref string) {
	if b.touched == nil {
		b.touched = map[string]bool{}
	}
	b.touched[ref] = true
}

func (b *sourceBook) add(s Source) *Source {
	for _, existing := range b.Items {
		if existing.Service == s.Service && existing.ObjectID == s.ObjectID && existing.Instance == s.Instance {
			existing.Revision = s.Revision
			b.touch(existing.Ref)
			return existing
		}
	}
	b.Next++
	s.Ref = "Q" + strconv.Itoa(b.Next)
	b.Items[s.Ref] = &s
	b.touch(s.Ref)
	return &s
}

// profileTools implements ToolBox for one authenticated profile.
type profileTools struct {
	hub       *Hub
	id        Identity
	sources   *sourceBook
	proposals []*Proposal
	defs      []toolDef
	media     mediaBackend
	docs      documentBackend
	photos    photoBackend
	language  *languagePack
	// model is the model of the run; tools are qualified for exactly it.
	model string
}

func (t *profileTools) lang() *languagePack {
	if t.language == nil {
		return languagePacks[defaultLanguage]
	}
	return t.language
}

type documentBackend interface {
	// Search returns the first hits and the total number of matches.
	Search(ctx context.Context, query string) ([]Document, int, error)
	// Containing matches term anywhere in title or text (compound words).
	Containing(ctx context.Context, term string) ([]Document, int, error)
	Read(ctx context.Context, id int) (Document, error)
}

type photoBackend interface {
	// Find returns the first hits and whether more match.
	Find(ctx context.Context, query, from, to string) ([]PhotoAsset, bool, error)
}

// hubDocuments and hubPhotos bind the adapters to one profile.
type hubDocuments struct {
	d  *Documents
	id Identity
}

func (h hubDocuments) Search(ctx context.Context, query string) ([]Document, int, error) {
	page, err := h.d.search(ctx, h.id, query, 1, 5)
	return page.Items, max(page.Count, len(page.Items)), err
}

func (h hubDocuments) Containing(ctx context.Context, term string) ([]Document, int, error) {
	page, err := h.d.containing(ctx, h.id, term, 5)
	return page.Items, max(page.Count, len(page.Items)), err
}

func (h hubDocuments) Read(ctx context.Context, id int) (Document, error) {
	return h.d.fetch(ctx, h.id, id, true)
}

type hubPhotos struct {
	p  *Photos
	id Identity
}

func (h hubPhotos) Find(ctx context.Context, query, from, to string) ([]PhotoAsset, bool, error) {
	items, more, _, err := h.p.findPage(ctx, h.id, photoQuery{Text: query, From: from, To: to}, 1, 12)
	return items, more, err
}

// mediaBackend isolates Jellyfin access so the casting fixture can exercise
// the identical harness and tool contract.
type mediaBackend interface {
	Movies(ctx context.Context) ([]Movie, error)
	Movie(ctx context.Context, id string) (Movie, error)
}

type Movie struct {
	ID       string
	Title    string
	Year     int
	Seconds  int
	Watched  bool
	Genres   []string
	Overview string
	Added    string
	Revision string
}

func movieTools() []toolDef {
	return []toolDef{
		newTool("search_movies", "Searches the movie library this profile may use. Returns title, year, runtime, genres and the source marker in the field source, but no plot description; call get_movie for that. All filters are optional and combined.", map[string]any{
			"query":                    map[string]any{"type": "string", "description": "Title or keyword from the title or description, in the user's language. Omit it to filter all movies. Years, runtimes and genres belong in their own filters."},
			"genre":                    map[string]any{"type": "string", "description": "Genre as named in the library, e.g. Komödie or Comedy."},
			"year":                     map[string]any{"type": "integer", "description": "Only movies released in this year, e.g. 2021."},
			"unwatched":                map[string]any{"type": "boolean", "description": "true = only movies not watched yet. Omit if it does not matter."},
			"watched":                  map[string]any{"type": "boolean", "description": "true = only movies already watched. Omit if it does not matter."},
			"runtime_under_minutes":    map[string]any{"type": "integer", "description": "For \"under N minutes\" or \"shorter than N minutes\": only movies shorter than N minutes. Two hours = 120."},
			"runtime_under_seconds":    map[string]any{"type": "integer", "description": "Like runtime_under_minutes, but for values in seconds."},
			"runtime_at_most_minutes":  map[string]any{"type": "integer", "description": "Only for \"at most/no more than/up to N minutes\" or \"N minutes or less\": movies up to and including N minutes."},
			"runtime_at_most_seconds":  map[string]any{"type": "integer", "description": "Like runtime_at_most_minutes, but for values in seconds."},
			"runtime_over_minutes":     map[string]any{"type": "integer", "description": "For \"longer than N minutes\" or \"over N minutes\": only movies longer than N minutes."},
			"runtime_over_seconds":     map[string]any{"type": "integer", "description": "Like runtime_over_minutes, but for values in seconds."},
			"runtime_at_least_minutes": map[string]any{"type": "integer", "description": "Only for \"at least N minutes\" or \"N minutes or more\": movies of N minutes or longer."},
			"runtime_at_least_seconds": map[string]any{"type": "integer", "description": "Like runtime_at_least_minutes, but for values in seconds."},
			"sort":                     map[string]any{"type": "string", "enum": []string{"runtime", "runtime_desc", "title", "year", "added"}, "description": "runtime = shortest first, runtime_desc = longest first, title = alphabetical, year = newest release first, added = most recently added first."},
			"limit":                    map[string]any{"type": "integer", "description": "Maximum number of hits, 1 to 10."},
		}),
		newTool("get_movie", "Reads details (description, year, runtime, genres) of a movie from an earlier result.", map[string]any{
			"source": map[string]any{"type": "string", "description": "Source marker such as Q1 from a result."},
		}, "source"),
		newTool("propose_favorite", "Proposes to mark a movie as favourite or to remove the mark. Changes nothing; the user confirms it.", map[string]any{
			"source":   map[string]any{"type": "string", "description": "Source marker of the movie, e.g. Q1."},
			"favorite": map[string]any{"type": "boolean", "description": "true = mark as favourite, false = remove the mark."},
		}, "source", "favorite"),
	}
}

func documentTools() []toolDef {
	return []toolDef{
		newTool("search_documents", "Full-text search in this profile's document archive. Returns title, date, a shortened text excerpt and the source marker. If the excerpt does not contain the answer, read the document with read_document.", map[string]any{
			"query": map[string]any{"type": "string", "description": "Search terms in the language of the documents, usually the user's language, e.g. Stadtwerke Rechnung."},
		}, "query"),
		newTool("read_document", "Reads the recognised text of a document from an earlier search result.", map[string]any{
			"source": map[string]any{"type": "string", "description": "Source marker such as Q2."},
		}, "source"),
	}
}

func photoTools() []toolDef {
	return []toolDef{
		newTool("search_photos", "Searches this profile's photos and private videos by description, file name, place and period.", map[string]any{
			"query": map[string]any{"type": "string", "description": "Search term in the user's language, e.g. See or Garten. Leave empty to search by period only."},
			"from":  map[string]any{"type": "string", "description": "Earliest capture date YYYY-MM-DD, optional."},
			"to":    map[string]any{"type": "string", "description": "Latest capture date YYYY-MM-DD, optional."},
		}),
	}
}

func (t *profileTools) Definitions() []toolDef { return t.defs }

// Issued reports whether the server handed out this marker in the conversation.
func (t *profileTools) Issued(ref string) bool { return t.sources.Items[ref] != nil }

// Titles lists the titles of the sources issued in this conversation.
func (t *profileTools) Titles() []string { return t.sources.titles() }

func (b *sourceBook) titles() []string {
	out := []string{}
	for _, s := range b.Items {
		// Photo titles are descriptions (untrusted data), and long texts are
		// no titles; copying them is still checked for the language.
		if s.Title != "" && s.Service != "photos" && len(s.Title) <= 80 {
			out = append(out, s.Title)
		}
	}
	// Longest first, so a title containing another one is removed whole.
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func toolJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func toolError(name, message string) ToolResult {
	return ToolResult{Content: toolJSON(map[string]string{"error": message}), Trace: ToolTrace{Name: name, Status: "failed", Summary: message}}
}

func formatRuntime(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d s", seconds)
	}
	if seconds < 3600 {
		if seconds%60 == 0 {
			return fmt.Sprintf("%d min", seconds/60)
		}
		return fmt.Sprintf("%d min %d s", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%d h %d min", seconds/3600, (seconds%3600)/60)
}

func (t *profileTools) source(ref string) *Source {
	s := t.sources.Items[strings.TrimSpace(strings.Trim(ref, "[]"))]
	if s != nil {
		// Reading an earlier source in this run makes the answer depend on it.
		t.sources.touch(s.Ref)
	}
	return s
}

// addSource registers a backend object with its current backend instance.
func (t *profileTools) addSource(s Source) *Source {
	if t.hub != nil {
		s.Instance = instanceOf(t.hub.store.Read(), s.Service)
	}
	return t.sources.add(s)
}

func (t *profileTools) Call(ctx context.Context, name string, raw json.RawMessage) ToolResult {
	allowed := false
	for _, d := range t.defs {
		if d.Function.Name == name {
			allowed = true
		}
	}
	if !allowed {
		return toolError(name, "This tool is not available.")
	}
	qualification := ""
	if t.hub != nil {
		var err error
		if t.model != "" {
			qualification, err = t.hub.ai.qualifyModel(toolTask(name), t.lang(), t.model)
		} else {
			qualification, err = t.hub.ai.qualify(toolTask(name), t.lang())
		}
		if err != nil {
			return toolError(name, "qualification_required")
		}
	}
	// A nil hub is the isolated synthetic casting fixture, never a product route.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	switch name {
	case "search_movies":
		var a struct {
			Query   string     `json:"query"`
			Genre   string     `json:"genre"`
			Year    flexNumber `json:"year"`
			Watched *bool      `json:"watched"`
			Unwatch *bool      `json:"unwatched"`
			// v4 names; the v3/v4-draft spellings stay accepted.
			Under        flexNumber `json:"runtime_under_seconds"`
			UnderMinutes flexNumber `json:"runtime_under_minutes"`
			Most         flexNumber `json:"runtime_at_most_seconds"`
			Over         flexNumber `json:"runtime_over_seconds"`
			OverMinutes  flexNumber `json:"runtime_over_minutes"`
			Least        flexNumber `json:"runtime_at_least_seconds"`
			LeastMinutes flexNumber `json:"runtime_at_least_minutes"`
			MostMinutes  flexNumber `json:"runtime_at_most_minutes"`
			Below        flexNumber `json:"runtime_below_seconds"`
			Minutes      flexNumber `json:"runtime_below_minutes"`
			MaxSeconds   flexNumber `json:"runtime_max_seconds"`
			MaxMinutes   flexNumber `json:"runtime_max_minutes"`
			Sort         string     `json:"sort"`
			Limit        flexNumber `json:"limit"`
		}
		if json.Unmarshal(raw, &a) != nil {
			return toolError(name, "Invalid arguments.")
		}
		movies, err := t.media.Movies(ctx)
		if err != nil {
			return toolError(name, "The movie library is currently unavailable.")
		}
		f := movieFilter{Query: a.Query, Genre: a.Genre, Year: int(a.Year), Watched: a.Watched, Sort: a.Sort, Limit: int(a.Limit)}
		switch {
		case a.Watched != nil && a.Unwatch != nil:
			// Both given: only a consistent pair filters. Defaults filled in
			// for every argument (false/false) or a contradiction do not.
			if *a.Watched == *a.Unwatch {
				f.Watched = nil
			}
		case a.Unwatch != nil:
			// An explicit false means the opposite filter, as in the v3 contract.
			seen := !*a.Unwatch
			f.Watched = &seen
		}
		f.Below = firstSeconds(a.Under, a.UnderMinutes, a.Below, a.Minutes)
		f.Max = firstSeconds(a.Most, a.MostMinutes, a.MaxSeconds, a.MaxMinutes)
		f.Above = firstSeconds(a.Over, a.OverMinutes)
		f.Min = firstSeconds(a.Least, a.LeastMinutes)
		hits, total := filterMovies(movies, f)
		out := []map[string]any{}
		for _, m := range hits {
			s := t.addSource(Source{Service: "media", Kind: "movie", ObjectID: m.ID, Title: m.Title, Subtitle: movieSubtitle(m),
				Facts: map[string]any{"seconds": m.Seconds}, Revision: m.Revision})
			out = append(out, map[string]any{"source": s.Ref, "title": m.Title, "year": m.Year, "runtime": formatRuntime(m.Seconds),
				"runtime_seconds": m.Seconds, "watched": m.Watched, "genres": m.Genres})
		}
		return ToolResult{Content: toolJSON(withTotal(map[string]any{"count": len(out), "hits": out}, len(out), total, "movies")),
			Trace: ToolTrace{Name: name, Status: "done", Summary: fmt.Sprintf(t.lang().found["movies"], total)}}
	case "get_movie":
		var a struct {
			Quelle string `json:"source"`
		}
		_ = json.Unmarshal(raw, &a)
		s := t.source(a.Quelle)
		if s == nil || s.Service != "media" {
			return toolError(name, "Unknown source marker.")
		}
		m, err := t.media.Movie(ctx, s.ObjectID)
		if err != nil {
			return toolError(name, "The movie is no longer available.")
		}
		overview := m.Overview
		if len(overview) > 800 {
			overview = overview[:800]
		}
		return ToolResult{Content: toolJSON(map[string]any{"source": s.Ref, "title": m.Title, "year": m.Year, "runtime": formatRuntime(m.Seconds),
			"runtime_seconds": m.Seconds, "watched": m.Watched, "genres": m.Genres, "description_data": overview}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: m.Title}}
	case "propose_favorite":
		var a struct {
			Quelle   string `json:"source"`
			Favorite *bool  `json:"favorite"`
		}
		_ = json.Unmarshal(raw, &a)
		s := t.source(a.Quelle)
		if s == nil || s.Service != "media" || a.Favorite == nil {
			return toolError(name, "Unknown source marker.")
		}
		p := &Proposal{ID: randomID(), Kind: "favorite", Ref: s.Ref, Favorite: *a.Favorite, State: "pending", Created: time.Now().UTC(),
			Expires: time.Now().Add(15 * time.Minute).UTC(), Device: t.id.Device, Qualification: qualification,
			Language: t.lang().Code}
		t.proposals = append(t.proposals, p)
		return ToolResult{Content: toolJSON(map[string]any{"proposal": "created", "status": "waiting for the user's confirmation; nothing has changed yet", "source": s.Ref}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: t.lang().found["proposal"]}}
	case "search_documents":
		var a struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(raw, &a) != nil || strings.TrimSpace(a.Query) == "" || len(a.Query) > 200 {
			return toolError(name, "Please provide a search term.")
		}
		items, total, err := t.docs.Search(ctx, a.Query)
		if err != nil {
			return toolError(name, "The document archive is currently unavailable.")
		}
		// Full-text search requires every term. If that finds nothing, look for
		// any of them, so one spelling variant does not hide the document.
		partial := false
		if words := strings.Fields(a.Query); len(items) == 0 && len(words) > 1 && !strings.Contains(a.Query, " OR ") {
			if items, total, err = t.docs.Search(ctx, strings.Join(words, " OR ")); err != nil {
				return toolError(name, "The document archive is currently unavailable.")
			}
			partial = len(items) > 0
		}
		// German compound words: "Rechnung" is not a word of "Arztrechnung"
		// in the full-text index. Look for the longest term inside words.
		inside := false
		if len(items) == 0 {
			words := slices.DeleteFunc(strings.Fields(strings.ReplaceAll(a.Query, " OR ", " ")), func(w string) bool { return utf8.RuneCountInString(w) < 4 })
			slices.SortStableFunc(words, func(x, y string) int { return utf8.RuneCountInString(y) - utf8.RuneCountInString(x) })
			for _, w := range words {
				if items, total, err = t.docs.Containing(ctx, w); err != nil {
					return toolError(name, "The document archive is currently unavailable.")
				}
				if len(items) > 0 {
					inside = true
					break
				}
			}
		}
		out := []map[string]any{}
		for _, d := range items {
			s := t.addSource(Source{Service: "documents", Kind: "document", ObjectID: strconv.Itoa(d.ID), Title: d.Title, Subtitle: d.Created, Revision: d.revision})
			out = append(out, map[string]any{"source": s.Ref, "title": d.Title, "date": d.Created, "excerpt_data": d.Snippet})
		}
		result := withTotal(map[string]any{"count": len(out), "hits": out}, len(out), total, "documents")
		if partial {
			result["note"] = "No document contains all terms; these hits contain some of them. Check that they answer the question."
		}
		if inside {
			result["note"] = "No document contains the terms as words; these hits contain one of them inside a longer word. Check that they answer the question."
		}
		return ToolResult{Content: toolJSON(result),
			Trace: ToolTrace{Name: name, Status: "done", Summary: fmt.Sprintf(t.lang().found["documents"], total)}}
	case "read_document":
		var a struct {
			Quelle string `json:"source"`
		}
		_ = json.Unmarshal(raw, &a)
		s := t.source(a.Quelle)
		if s == nil || s.Service != "documents" {
			return toolError(name, "Unknown source marker.")
		}
		n, _ := strconv.Atoi(s.ObjectID)
		d, err := t.docs.Read(ctx, n)
		if err != nil {
			return toolError(name, "The document is not available.")
		}
		text := *d.Content
		if len(text) > 6000 {
			text = text[:6000] + " …"
		}
		return ToolResult{Content: toolJSON(map[string]any{"source": s.Ref, "title": d.Title, "date": d.Created, "text_data": text}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: d.Title}}
	case "search_photos":
		var a struct {
			Query string `json:"query"`
			From  string `json:"from"`
			To    string `json:"to"`
		}
		if json.Unmarshal(raw, &a) != nil || len(a.Query) > 200 || (a.From != "" && !dateParam.MatchString(a.From)) || (a.To != "" && !dateParam.MatchString(a.To)) {
			return toolError(name, "Invalid arguments.")
		}
		items, more, err := t.photos.Find(ctx, a.Query, a.From, a.To)
		if err != nil {
			return toolError(name, "The photo library is currently unavailable.")
		}
		// Without smart search the library compares the whole query with
		// descriptions, so "lake sunset" misses "Lake at sunset" and
		// "skateboarding" misses "skateboard". Look for the words, then
		// their stems, before reporting nothing.
		fallback := ""
		if len(items) == 0 && strings.TrimSpace(a.Query) != "" {
			if items, more, fallback, err = t.photoFallback(ctx, a.Query, a.From, a.To); err != nil {
				return toolError(name, "The photo library is currently unavailable.")
			}
		}
		out := []map[string]any{}
		for _, p := range items {
			title := p.Description
			if title == "" {
				title = p.FileName
			}
			s := t.addSource(Source{Service: "photos", Kind: p.Type, ObjectID: p.ID, Title: title, Subtitle: dateOnly(p.Taken), Revision: p.revision})
			out = append(out, map[string]any{"source": s.Ref, "kind": p.Type, "taken": dateOnly(p.Taken), "description_data": p.Description, "place": p.City})
		}
		result := map[string]any{"count": len(out), "hits": out}
		if fallback != "" {
			result["note"] = fallback
		}
		if more {
			result["more"] = true
			result["note"] = strings.TrimSpace(fallback + " " + fmt.Sprintf("More photos match; only the first %d are listed. Do not present them as all photos or as a total number.", len(out)))
		}
		return ToolResult{Content: toolJSON(result),
			Trace: ToolTrace{Name: name, Status: "done", Summary: fmt.Sprintf(t.lang().found["photos"], len(out))}}
	}
	return toolError(name, "Unknown tool.")
}

// photoFallback searches the significant words of a query that found
// nothing: photos matching every word, else any word, else a word stem.
func (t *profileTools) photoFallback(ctx context.Context, query, from, to string) ([]PhotoAsset, bool, string, error) {
	var words []string
	for _, w := range searchWord.FindAllString(query, -1) {
		if lw := strings.ToLower(w); utf8.RuneCountInString(lw) >= 3 && !t.lang().stop[lw] {
			words = append(words, lw)
		}
	}
	search := func(terms []string) (all, any []PhotoAsset, more bool, err error) {
		count := map[string]int{}
		seen := map[string]bool{}
		for _, term := range terms {
			items, m, err := t.photos.Find(ctx, term, from, to)
			if err != nil {
				return nil, nil, false, err
			}
			more = more || m
			for _, p := range items {
				count[p.ID]++
				if !seen[p.ID] {
					seen[p.ID] = true
					any = append(any, p)
				}
			}
		}
		for _, p := range any {
			if count[p.ID] == len(terms) {
				all = append(all, p)
			}
		}
		return all, any, more, nil
	}
	if len(words) > 1 {
		all, any, more, err := search(words)
		if err != nil || len(all) > 0 {
			return all, more, "No photo matches the whole query; these match each of its words.", err
		}
		if len(any) > 0 {
			return any, more, "No photo matches the whole query; these match some of its words. Check that they answer the question.", nil
		}
	}
	var stems []string
	for _, w := range words {
		if s := stem(w); s != w {
			stems = append(stems, s)
		}
	}
	if len(stems) > 0 {
		_, any, more, err := search(stems)
		if err != nil || len(any) > 0 {
			return any, more, "No photo matches the words as written; these match a shorter form of them. Check that they answer the question.", err
		}
	}
	return nil, false, "", nil
}

// stem removes a common English or German ending ("skateboarding" ->
// "skateboard", "Gärten" -> "gärt") so a description with the base form
// matches; the result keeps at least four letters.
func stem(word string) string {
	for _, suffix := range []string{"ing", "ern", "en", "er", "es", "ed", "s", "e", "n"} {
		if r := strings.TrimSuffix(word, suffix); r != word && utf8.RuneCountInString(r) >= 4 {
			return r
		}
	}
	return word
}

// withTotal adds the number of all matches when only the first ones are
// listed, so "how many …" is answered from the total, not the list length.
func withTotal(result map[string]any, listed, total int, what string) map[string]any {
	if total > listed {
		result["total"] = total
		result["note"] = fmt.Sprintf("%d %s match in total; only the first %d are listed. For a number use total; do not present the list as complete.", total, what, listed)
	}
	return result
}

// flexNumber accepts 5 and "5": some models quote numbers. An empty string
// counts as not given.
type flexNumber float64

func (n *flexNumber) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err == nil {
		*n = flexNumber(f)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s = strings.TrimSpace(s); s == "" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*n = flexNumber(f)
	return nil
}

// firstSeconds picks the first given bound from (seconds, minutes) pairs.
func firstSeconds(pairs ...flexNumber) int {
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i] > 0 {
			return int(pairs[i])
		}
		if pairs[i+1] > 0 {
			return int(pairs[i+1] * 60)
		}
	}
	return 0
}

func dateOnly(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func movieSubtitle(m Movie) string {
	parts := []string{}
	if m.Year > 0 {
		parts = append(parts, strconv.Itoa(m.Year))
	}
	if m.Seconds > 0 {
		parts = append(parts, formatRuntime(m.Seconds))
	}
	return strings.Join(parts, " · ")
}

// movieFilter combines all search_movies filters. Below is exclusive, Max is
// inclusive; both are seconds.
type movieFilter struct {
	Query, Genre, Sort string
	Year, Below, Max   int
	// Above is exclusive ("longer than"), Min inclusive ("at least").
	Above, Min int
	Watched    *bool
	Limit      int
}

func filterMovies(movies []Movie, f movieFilter) ([]Movie, int) {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	g := strings.ToLower(strings.TrimSpace(f.Genre))
	below, limit, order := f.Below, f.Limit, f.Sort
	out := []Movie{}
	for _, m := range movies {
		if q != "" && !strings.Contains(strings.ToLower(m.Title), q) && !strings.Contains(strings.ToLower(m.Overview), q) {
			continue
		}
		if below > 0 && (m.Seconds <= 0 || m.Seconds >= below) {
			continue
		}
		if f.Above > 0 && m.Seconds <= f.Above {
			continue
		}
		if f.Min > 0 && m.Seconds < f.Min {
			continue
		}
		if f.Max > 0 && (m.Seconds <= 0 || m.Seconds > f.Max) {
			continue
		}
		if f.Year > 0 && m.Year != f.Year {
			continue
		}
		if f.Watched != nil && *f.Watched != m.Watched {
			continue
		}
		if g != "" {
			match := false
			for _, mg := range m.Genres {
				if strings.Contains(strings.ToLower(mg), g) {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		switch order {
		case "runtime":
			// Unknown runtimes (0) are not the shortest; they come last.
			a, b := out[i].Seconds, out[j].Seconds
			return a > 0 && (b <= 0 || a < b)
		case "runtime_desc":
			return out[i].Seconds > out[j].Seconds
		case "year":
			return out[i].Year > out[j].Year
		case "added":
			return out[i].Added > out[j].Added
		default:
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
	})
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	total := len(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total
}

// jellyfinMedia reads movies with the profile's own Jellyfin session, so
// library rights are enforced by Jellyfin itself.
type jellyfinMedia struct {
	jf *jellyfin
	id Identity
}

type jellyItem struct {
	ID             string   `json:"Id"`
	Name           string   `json:"Name"`
	ProductionYear int      `json:"ProductionYear"`
	RunTimeTicks   int64    `json:"RunTimeTicks"`
	Genres         []string `json:"Genres"`
	Overview       string   `json:"Overview"`
	DateCreated    string   `json:"DateCreated"`
	PremiereDate   string   `json:"PremiereDate"`
	Type           string   `json:"Type"`
	Etag           string   `json:"Etag"`
	UserData       struct {
		Played     bool `json:"Played"`
		IsFavorite bool `json:"IsFavorite"`
	} `json:"UserData"`
}

func (j jellyItem) movie() Movie {
	return Movie{ID: normalizeID(j.ID), Title: j.Name, Year: j.ProductionYear, Seconds: int(j.RunTimeTicks / 10_000_000), Watched: j.UserData.Played,
		Genres: j.Genres, Overview: j.Overview, Added: j.DateCreated, Revision: j.Etag}
}

func (m jellyfinMedia) Movies(ctx context.Context) ([]Movie, error) {
	var out struct {
		Items []jellyItem `json:"Items"`
	}
	params := url.Values{"IncludeItemTypes": {"Movie"}, "Recursive": {"true"}, "Fields": {"Genres,Overview,DateCreated,Etag"}, "Limit": {"2000"}, "EnableImages": {"false"}}
	if err := m.jf.call(ctx, "GET", "/Users/"+m.id.UserID+"/Items?"+params.Encode(), m.id.token, nil, &out); err != nil {
		return nil, err
	}
	movies := make([]Movie, 0, len(out.Items))
	for _, it := range out.Items {
		movies = append(movies, it.movie())
	}
	return movies, nil
}

func (m jellyfinMedia) Movie(ctx context.Context, id string) (Movie, error) {
	var it jellyItem
	if err := m.jf.call(ctx, "GET", "/Users/"+m.id.UserID+"/Items/"+url.PathEscape(id), m.id.token, nil, &it); err != nil {
		return Movie{}, err
	}
	return it.movie(), nil
}
