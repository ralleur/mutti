// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
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
}

// sourceBook allocates conversation-scoped markers. A run works on a copy and
// the result is merged when the run is stored.
type sourceBook struct {
	Items map[string]*Source `json:"items"`
	Next  int                `json:"next"`
}

func (b *sourceBook) add(s Source) *Source {
	for _, existing := range b.Items {
		if existing.Service == s.Service && existing.ObjectID == s.ObjectID {
			return existing
		}
	}
	b.Next++
	s.Ref = "Q" + strconv.Itoa(b.Next)
	b.Items[s.Ref] = &s
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
}

func movieTools() []toolDef {
	return []toolDef{
		newTool("search_movies", "Durchsucht die für dieses Profil freigegebene Filmbibliothek. Liefert Treffer mit Quellenmarke im Feld quelle.", map[string]any{
			"query":                 map[string]any{"type": "string", "description": "Optionaler Titel oder Stichwort. Weglassen, um alle Filme zu filtern."},
			"runtime_below_seconds": map[string]any{"type": "integer", "description": "Nur Filme, die kürzer sind als dieser Wert in Sekunden (exklusive Grenze). \"unter 100 Sekunden\" ergibt 100, \"unter 90 Minuten\" ergibt 5400, \"höchstens 90 Minuten\" ergibt 5401."},
			"unwatched":             map[string]any{"type": "boolean", "description": "true = nur noch nicht gesehene Filme. Weglassen, wenn egal."},
			"genre":                 map[string]any{"type": "string", "description": "Optionales Genre, z. B. Komödie."},
			"sort":                  map[string]any{"type": "string", "enum": []string{"runtime", "title", "year", "added"}, "description": "Sortierung; Laufzeit aufsteigend bei runtime."},
			"limit":                 map[string]any{"type": "integer", "description": "Maximale Trefferzahl, 1 bis 10."},
		}),
		newTool("get_movie", "Liest Details (Beschreibung, Jahr, Laufzeit, Genres) eines Films aus einem früheren Ergebnis.", map[string]any{
			"quelle": map[string]any{"type": "string", "description": "Quellenmarke wie Q1 aus einem Ergebnis."},
		}, "quelle"),
		newTool("propose_favorite", "Schlägt vor, einen Film als Favorit zu markieren oder die Markierung zu entfernen. Ändert nichts; der Nutzer bestätigt selbst.", map[string]any{
			"quelle":   map[string]any{"type": "string", "description": "Quellenmarke des Films, z. B. Q1."},
			"favorite": map[string]any{"type": "boolean", "description": "true = als Favorit markieren, false = Markierung entfernen."},
		}, "quelle", "favorite"),
	}
}

func documentTools() []toolDef {
	return []toolDef{
		newTool("search_documents", "Volltextsuche im Dokumentenarchiv dieses Profils. Liefert Titel, Datum, Textauszug und Quellenmarke.", map[string]any{
			"query": map[string]any{"type": "string", "description": "Suchbegriffe, z. B. Stadtwerke Rechnung."},
		}, "query"),
		newTool("read_document", "Liest den erkannten Text eines Dokuments aus einem früheren Suchergebnis.", map[string]any{
			"quelle": map[string]any{"type": "string", "description": "Quellenmarke wie Q2."},
		}, "quelle"),
	}
}

func photoTools() []toolDef {
	return []toolDef{
		newTool("search_photos", "Sucht Fotos und private Videos dieses Profils nach Beschreibung, Dateiname, Ort und Zeitraum.", map[string]any{
			"query": map[string]any{"type": "string", "description": "Suchbegriff, z. B. See oder Garten. Leer lassen für nur Zeitraum."},
			"from":  map[string]any{"type": "string", "description": "Frühestes Aufnahmedatum JJJJ-MM-TT, optional."},
			"to":    map[string]any{"type": "string", "description": "Spätestes Aufnahmedatum JJJJ-MM-TT, optional."},
		}),
	}
}

func (t *profileTools) Definitions() []toolDef { return t.defs }

func toolJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func toolError(name, message string) ToolResult {
	return ToolResult{Content: toolJSON(map[string]string{"fehler": message}), Trace: ToolTrace{Name: name, Status: "failed", Summary: message}}
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
	return t.sources.Items[strings.TrimSpace(strings.Trim(ref, "[]"))]
}

func (t *profileTools) Call(ctx context.Context, name string, raw json.RawMessage) ToolResult {
	allowed := false
	for _, d := range t.defs {
		if d.Function.Name == name {
			allowed = true
		}
	}
	if !allowed {
		return toolError(name, "Dieses Werkzeug ist nicht verfügbar.")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	switch name {
	case "search_movies":
		var a struct {
			Query   string  `json:"query"`
			Below   float64 `json:"runtime_below_seconds"`
			Unwatch *bool   `json:"unwatched"`
			Genre   string  `json:"genre"`
			Sort    string  `json:"sort"`
			Limit   float64 `json:"limit"`
		}
		if json.Unmarshal(raw, &a) != nil {
			return toolError(name, "Ungültige Argumente.")
		}
		movies, err := t.media.Movies(ctx)
		if err != nil {
			return toolError(name, "Die Filmbibliothek ist gerade nicht verfügbar.")
		}
		hits := filterMovies(movies, a.Query, int(a.Below), a.Unwatch, a.Genre, a.Sort, int(a.Limit))
		out := []map[string]any{}
		for _, m := range hits {
			s := t.sources.add(Source{Service: "media", Kind: "movie", ObjectID: m.ID, Title: m.Title, Subtitle: movieSubtitle(m),
				Facts: map[string]any{"seconds": m.Seconds}})
			out = append(out, map[string]any{"quelle": s.Ref, "titel": m.Title, "jahr": m.Year, "laufzeit": formatRuntime(m.Seconds),
				"laufzeit_sekunden": m.Seconds, "gesehen": m.Watched, "genres": m.Genres})
		}
		return ToolResult{Content: toolJSON(map[string]any{"anzahl": len(out), "treffer": out}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: fmt.Sprintf("%d Filme gefunden", len(out))}}
	case "get_movie":
		var a struct {
			Quelle string `json:"quelle"`
		}
		_ = json.Unmarshal(raw, &a)
		s := t.source(a.Quelle)
		if s == nil || s.Service != "media" {
			return toolError(name, "Unbekannte Quellenmarke.")
		}
		m, err := t.media.Movie(ctx, s.ObjectID)
		if err != nil {
			return toolError(name, "Der Film ist nicht mehr verfügbar.")
		}
		overview := m.Overview
		if len(overview) > 800 {
			overview = overview[:800]
		}
		return ToolResult{Content: toolJSON(map[string]any{"quelle": s.Ref, "titel": m.Title, "jahr": m.Year, "laufzeit": formatRuntime(m.Seconds),
			"laufzeit_sekunden": m.Seconds, "gesehen": m.Watched, "genres": m.Genres, "beschreibung_daten": overview}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: m.Title}}
	case "propose_favorite":
		var a struct {
			Quelle   string `json:"quelle"`
			Favorite *bool  `json:"favorite"`
		}
		_ = json.Unmarshal(raw, &a)
		s := t.source(a.Quelle)
		if s == nil || s.Service != "media" || a.Favorite == nil {
			return toolError(name, "Unbekannte Quellenmarke.")
		}
		p := &Proposal{ID: randomID(), Kind: "favorite", Ref: s.Ref, Favorite: *a.Favorite, State: "pending", Created: time.Now().UTC(),
			Expires: time.Now().Add(15 * time.Minute).UTC(), Device: t.id.Device}
		t.proposals = append(t.proposals, p)
		return ToolResult{Content: toolJSON(map[string]any{"vorschlag": "angelegt", "status": "wartet auf Bestätigung durch den Nutzer; noch nichts geändert", "quelle": s.Ref}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: "Vorschlag wartet auf Bestätigung"}}
	case "search_documents":
		var a struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(raw, &a) != nil || strings.TrimSpace(a.Query) == "" || len(a.Query) > 200 {
			return toolError(name, "Bitte einen Suchbegriff angeben.")
		}
		page, err := t.hub.docs.search(ctx, t.id, a.Query, 1, 5)
		if err != nil {
			return toolError(name, "Das Dokumentenarchiv ist gerade nicht verfügbar.")
		}
		out := []map[string]any{}
		for _, d := range page.Items {
			s := t.sources.add(Source{Service: "documents", Kind: "document", ObjectID: strconv.Itoa(d.ID), Title: d.Title, Subtitle: d.Created})
			out = append(out, map[string]any{"quelle": s.Ref, "titel": d.Title, "datum": d.Created, "auszug_daten": d.Snippet})
		}
		return ToolResult{Content: toolJSON(map[string]any{"anzahl": len(out), "treffer": out}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: fmt.Sprintf("%d Dokumente gefunden", len(out))}}
	case "read_document":
		var a struct {
			Quelle string `json:"quelle"`
		}
		_ = json.Unmarshal(raw, &a)
		s := t.source(a.Quelle)
		if s == nil || s.Service != "documents" {
			return toolError(name, "Unbekannte Quellenmarke.")
		}
		n, _ := strconv.Atoi(s.ObjectID)
		d, err := t.hub.docs.fetch(ctx, t.id, n, true)
		if err != nil {
			return toolError(name, "Das Dokument ist nicht verfügbar.")
		}
		text := *d.Content
		if len(text) > 6000 {
			text = text[:6000] + " …"
		}
		return ToolResult{Content: toolJSON(map[string]any{"quelle": s.Ref, "titel": d.Title, "datum": d.Created, "text_daten": text}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: d.Title}}
	case "search_photos":
		var a struct {
			Query string `json:"query"`
			From  string `json:"from"`
			To    string `json:"to"`
		}
		if json.Unmarshal(raw, &a) != nil || len(a.Query) > 200 || (a.From != "" && !dateParam.MatchString(a.From)) || (a.To != "" && !dateParam.MatchString(a.To)) {
			return toolError(name, "Ungültige Argumente.")
		}
		items, _, err := t.hub.photos.find(ctx, t.id, a.Query, a.From, a.To, 12)
		if err != nil {
			return toolError(name, "Die Fotobibliothek ist gerade nicht verfügbar.")
		}
		out := []map[string]any{}
		for _, p := range items {
			title := p.Description
			if title == "" {
				title = p.FileName
			}
			s := t.sources.add(Source{Service: "photos", Kind: p.Type, ObjectID: p.ID, Title: title, Subtitle: dateOnly(p.Taken)})
			out = append(out, map[string]any{"quelle": s.Ref, "art": p.Type, "aufgenommen": dateOnly(p.Taken), "beschreibung_daten": p.Description, "ort": p.City})
		}
		return ToolResult{Content: toolJSON(map[string]any{"anzahl": len(out), "treffer": out}),
			Trace: ToolTrace{Name: name, Status: "done", Summary: fmt.Sprintf("%d Fotos/Videos gefunden", len(out))}}
	}
	return toolError(name, "Unbekanntes Werkzeug.")
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

func filterMovies(movies []Movie, query string, below int, unwatched *bool, genre, order string, limit int) []Movie {
	q := strings.ToLower(strings.TrimSpace(query))
	g := strings.ToLower(strings.TrimSpace(genre))
	out := []Movie{}
	for _, m := range movies {
		if q != "" && !strings.Contains(strings.ToLower(m.Title), q) && !strings.Contains(strings.ToLower(m.Overview), q) {
			continue
		}
		if below > 0 && (m.Seconds <= 0 || m.Seconds >= below) {
			continue
		}
		if unwatched != nil && *unwatched == m.Watched {
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
			return out[i].Seconds < out[j].Seconds
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
	if len(out) > limit {
		out = out[:limit]
	}
	return out
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
	UserData       struct {
		Played     bool `json:"Played"`
		IsFavorite bool `json:"IsFavorite"`
	} `json:"UserData"`
}

func (j jellyItem) movie() Movie {
	return Movie{ID: normalizeID(j.ID), Title: j.Name, Year: j.ProductionYear, Seconds: int(j.RunTimeTicks / 10_000_000), Watched: j.UserData.Played,
		Genres: j.Genres, Overview: j.Overview, Added: j.DateCreated}
}

func (m jellyfinMedia) Movies(ctx context.Context) ([]Movie, error) {
	var out struct {
		Items []jellyItem `json:"Items"`
	}
	params := url.Values{"IncludeItemTypes": {"Movie"}, "Recursive": {"true"}, "Fields": {"Genres,Overview,DateCreated"}, "Limit": {"2000"}, "EnableImages": {"false"}}
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
