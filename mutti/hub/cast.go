// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CastOptions runs the frozen casting fixture through the product harness:
// same prompt, tool definitions, source markers and confined engine.
type CastOptions struct {
	Fixture, Rubric, Output, Store, Ollama string
	Models                                 []string
	Repetitions                            int
}

type castFixture struct {
	Version string `json:"version"`
	Prompt  string `json:"prompt"`
	// Language is the user's language of this suite (answer language).
	Language    string   `json:"language"`
	Speculation []string `json:"speculation"`
	Data        struct {
		Movies []struct {
			Key, Title, Added, Overview string
			Year, Seconds               int
			Watched                     bool
			Genres                      []string
		} `json:"movies"`
		Documents []struct {
			Key, Title, Created, Text string
			ID                        int
		} `json:"documents"`
		Photos []struct {
			Key, ID, Type, Taken, Description, City string
		} `json:"photos"`
	} `json:"data"`
	Cases []castCase `json:"cases"`
}

type castCase struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Prompt   string `json:"prompt"`
	History  []struct {
		Role, Text string
		Sources    []string
	} `json:"history"`
	Attachment *struct{ Name, Text string } `json:"attachment"`
	FailTools  []string                     `json:"fail_tools"`
	Checks     struct {
		Calls []struct {
			Name         string           `json:"name"`
			Args         map[string]any   `json:"args"`
			AllowExtra   []string         `json:"allow_extra"`
			Alternatives []map[string]any `json:"alternatives"`
		} `json:"calls"`
		NoCalls          bool     `json:"no_calls"`
		ForbidCalls      []string `json:"forbid_calls"`
		ForbidQueries    []string `json:"forbid_queries"`
		AnswerAll        []string `json:"answer_all"`
		AnswerAny        []string `json:"answer_any"`
		AnswerAny2       []string `json:"answer_any2"`
		AnswerNone       []string `json:"answer_none"`
		TitlesAll        []string `json:"titles_all"`
		TitlesOnly       bool     `json:"titles_only"`
		TitlesNone       []string `json:"titles_none"`
		Cite             []string `json:"cite"`
		NoCitations      bool     `json:"no_citations"`
		Question         bool     `json:"question"`
		NoForeignMarkers bool     `json:"no_foreign_markers"`
		German           bool     `json:"german"` // legacy name of language
		Language         bool     `json:"language"`
	} `json:"checks"`
}

type castData struct {
	f    *castFixture
	fail map[string]bool
}

func (d castData) Movies(context.Context) ([]Movie, error) {
	if d.fail["search_movies"] {
		return nil, errors.New("synthetic outage")
	}
	out := []Movie{}
	for _, m := range d.f.Data.Movies {
		out = append(out, Movie{ID: m.Key, Title: m.Title, Year: m.Year, Seconds: m.Seconds, Watched: m.Watched, Genres: m.Genres, Overview: m.Overview, Added: m.Added})
	}
	return out, nil
}

func (d castData) Movie(ctx context.Context, id string) (Movie, error) {
	movies, _ := castData{f: d.f}.Movies(ctx)
	for _, m := range movies {
		if m.ID == id {
			return m, nil
		}
	}
	return Movie{}, errNotFound
}

var castWord = regexp.MustCompile(`[\p{L}\p{N}-]{3,}`)

func (d castData) Search(_ context.Context, query string) ([]Document, int, error) {
	if d.fail["search_documents"] {
		return nil, 0, errors.New("synthetic outage")
	}
	type hit struct {
		score int
		doc   Document
	}
	hits := []hit{}
	for _, doc := range d.f.Data.Documents {
		hay := strings.ToLower(doc.Title + " " + doc.Text)
		score := 0
		first := -1
		for _, w := range castWord.FindAllString(strings.ToLower(query), -1) {
			if i := strings.Index(hay, w); i >= 0 {
				score++
				if first < 0 || i < first {
					first = i
				}
			}
		}
		if score == 0 {
			continue
		}
		body := doc.Title + " " + doc.Text
		start := max(0, first-60)
		end := min(len(body), start+180)
		snippet := strings.ToValidUTF8(body[start:end], "")
		hits = append(hits, hit{score, Document{ID: doc.ID, Title: doc.Title, Created: doc.Created, Snippet: snippet}})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := []Document{}
	for i, h := range hits {
		if i == 5 {
			break
		}
		out = append(out, h.doc)
	}
	return out, len(hits), nil
}

func (d castData) Read(_ context.Context, id int) (Document, error) {
	for _, doc := range d.f.Data.Documents {
		if doc.ID == id {
			text := doc.Text
			return Document{ID: doc.ID, Title: doc.Title, Created: doc.Created, Content: &text}, nil
		}
	}
	return Document{}, errNotFound
}

func (d castData) Find(_ context.Context, query, from, to string) ([]PhotoAsset, bool, error) {
	out := []PhotoAsset{}
	q := strings.ToLower(query)
	for _, p := range d.f.Data.Photos {
		if q != "" && !strings.Contains(strings.ToLower(p.Description+" "+p.City), q) {
			continue
		}
		if (from != "" && p.Taken < from) || (to != "" && p.Taken > to) {
			continue
		}
		out = append(out, PhotoAsset{ID: p.ID, Type: p.Type, Taken: p.Taken, Description: p.Description, City: p.City})
	}
	if len(out) > 12 {
		return out[:12], true, nil
	}
	return out, false, nil
}

// objectRef maps fixture keys like "movie:nordlicht" to the server marker.
func (d castData) objectRef(book *sourceBook, key string) string {
	kind, name, _ := strings.Cut(key, ":")
	for _, s := range book.Items {
		switch {
		case kind == "movie" && s.Service == "media" && s.ObjectID == name:
			return s.Ref
		case kind == "document" && s.Service == "documents":
			for _, doc := range d.f.Data.Documents {
				if doc.Key == name && strconv.Itoa(doc.ID) == s.ObjectID {
					return s.Ref
				}
			}
		case kind == "attachment" && s.Service == "attachment":
			return s.Ref
		}
	}
	return ""
}

type castCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"arguments"`
}

type castResult struct {
	Model        string         `json:"model"`
	Repeat       int            `json:"repeat"`
	Case         string         `json:"case"`
	Category     string         `json:"category"`
	Passed       bool           `json:"passed"`
	Failures     []string       `json:"failures,omitempty"`
	Answer       string         `json:"answer"`
	Raw          string         `json:"raw"`
	Calls        []castCall     `json:"calls"`
	Cited        []string       `json:"cited"`
	Invalid      int            `json:"invalidMarkers"`
	FirstSeconds float64        `json:"firstSeconds"`
	Seconds      float64        `json:"seconds"`
	EvalTokens   int            `json:"evalTokens"`
	TokensPerSec float64        `json:"tokensPerSecond"`
	Truncated    bool           `json:"truncated"`
	Error        string         `json:"error,omitempty"`
	Rounds       int            `json:"rounds"`
	Guarded      bool           `json:"guarded"`
	Corrections  []string       `json:"interventions,omitempty"`
	Sources      map[string]any `json:"sources"`
}

// recordingTools captures calls for scoring while delegating to the product tools.
type recordingTools struct {
	*profileTools
	calls []castCall
}

func (r *recordingTools) Call(ctx context.Context, name string, args json.RawMessage) ToolResult {
	r.calls = append(r.calls, castCall{name, args})
	return r.profileTools.Call(ctx, name, args)
}

func fileHash(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func RunCasting(o CastOptions) error {
	if _, err := os.Stat(o.Output); err == nil {
		return errors.New("use a new result directory")
	}
	if err := os.MkdirAll(o.Output, 0700); err != nil {
		return err
	}
	raw, err := os.ReadFile(o.Fixture)
	if err != nil {
		return err
	}
	var f castFixture
	if err = json.Unmarshal(raw, &f); err != nil || f.Prompt != PromptVersion {
		return errors.New("fixture does not match the product prompt version")
	}
	lang, ok := languageFor(f.Language)
	if !ok {
		return errors.New("fixture has no supported language")
	}
	fixtureHash, _ := fileHash(o.Fixture)
	rubricHash, err := fileHash(o.Rubric)
	if err != nil {
		return err
	}
	engineHash, _ := fileHash(o.Ollama)
	e := newEngine(o.Store, o.Ollama, true)
	ctx := context.Background()
	if err = e.ensure(ctx); err != nil {
		return err
	}
	defer e.stop()
	base, _ := e.base()
	var version struct {
		Version string `json:"version"`
	}
	_ = serviceCall(ctx, e.client, "GET", base+"/api/version", nil, nil, &version)
	env := map[string]any{"started": time.Now().UTC(), "fixture": f.Version, "fixtureSha256": fixtureHash, "rubricSha256": rubricHash,
		"prompt": PromptVersion, "language": lang.Code, "engine": version.Version, "engineSha256": engineHash, "networkLock": e.status().NetworkLock,
		"settings": map[string]any{"num_ctx": 8192, "num_predict": 1024, "temperature": 0, "seed": 42, "thinking": false, "rounds": 4},
		"memoryGB": systemMemoryGB(), "os": runtime.GOOS + "/" + runtime.GOARCH, "models": []any{}}
	if out, err := exec.Command("/usr/sbin/sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		env["hardware"] = strings.TrimSpace(string(out))
	}
	writeEnv := func() {
		b, _ := json.MarshalIndent(env, "", "  ")
		_ = os.WriteFile(filepath.Join(o.Output, "environment.json"), b, 0600)
	}
	results, err := os.OpenFile(filepath.Join(o.Output, "results.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer results.Close()
	all := []castResult{}
	for _, id := range o.Models {
		model, ok := catalogModel(id)
		if !ok || !e.verified(ctx, model) {
			return fmt.Errorf("model %s is not the pinned catalog artifact in this store", id)
		}
		env["models"] = append(env["models"].([]any), model)
		writeEnv()
		off := false
		h := &Harness{Client: e.stream, Base: base, Model: model.ID, Rounds: 4, Lang: lang,
			Options: map[string]any{"num_ctx": 8192, "temperature": 0, "seed": 42, "num_predict": 1024}}
		if h.Think, err = (&AI{engine: e, show: map[string]bool{}}).thinkFlag(ctx, base, model.ID, &off); err != nil {
			return err
		}
		for rep := 1; rep <= o.Repetitions; rep++ {
			for _, c := range f.Cases {
				r := castOne(ctx, h, &f, c, model)
				r.Repeat = rep
				all = append(all, r)
				b, _ := json.Marshal(r)
				_, _ = results.Write(append(b, '\n'))
				fmt.Printf("%s r%d %s %v %.1fs %s\n", model.ID, rep, c.ID, r.Passed, r.Seconds, strings.Join(r.Failures, "; "))
			}
		}
		_ = serviceCall(ctx, e.client, "POST", base+"/api/generate", nil, map[string]any{"model": model.ID, "keep_alive": 0}, nil)
	}
	env["finished"] = time.Now().UTC()
	writeEnv()
	summary := summarize(all)
	b, _ := json.MarshalIndent(summary, "", "  ")
	return os.WriteFile(filepath.Join(o.Output, "summary.json"), b, 0600)
}

var cjk = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}\p{Cyrillic}\p{Arabic}]`)

func castOne(ctx context.Context, h *Harness, f *castFixture, c castCase, model CatalogModel) castResult {
	fail := map[string]bool{}
	for _, t := range c.FailTools {
		fail[t] = true
	}
	data := castData{f: f, fail: fail}
	book := &sourceBook{Items: map[string]*Source{}}
	tools := &recordingTools{profileTools: &profileTools{sources: book, media: data, docs: data, photos: data, language: h.Lang,
		defs: append(append(movieTools(), documentTools()...), photoTools()...)}}
	msgs := []chatMessage{{Role: "system", Content: SystemPrompt(model.Name, time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), tools.defs, h.Lang)}}
	for _, m := range c.History {
		content := m.Text
		if len(m.Sources) > 0 {
			refs := []string{}
			for _, key := range m.Sources {
				kind, name, _ := strings.Cut(key, ":")
				if kind == "movie" {
					movie, _ := data.Movie(ctx, name)
					refs = append(refs, book.add(Source{Service: "media", Kind: "movie", ObjectID: movie.ID, Title: movie.Title, Subtitle: movieSubtitle(movie)}).Ref)
				}
			}
			content += "\n\n(Sources of this answer: " + bookMemo(book, refs) + ")"
		}
		msgs = append(msgs, chatMessage{Role: m.Role, Content: content})
	}
	prompt := c.Prompt
	if c.Attachment != nil {
		s := book.add(Source{Service: "attachment", Kind: "attachment", ObjectID: "fixture-attachment", Title: c.Attachment.Name})
		prompt += attachmentBlock(c.Attachment.Name, s.Ref, c.Attachment.Text)
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: prompt})
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	start := time.Now()
	res, err := h.Run(runCtx, msgs, tools, func(HarnessEvent) {})
	r := castResult{Model: model.ID, Case: c.ID, Category: c.Category, Raw: res.Text, Calls: tools.calls, Seconds: time.Since(start).Seconds(),
		FirstSeconds: res.FirstToken.Seconds(), EvalTokens: res.EvalTokens, Truncated: res.Truncated, Rounds: res.Rounds, Guarded: res.Guarded,
		Corrections: res.Interventions, Sources: map[string]any{}}
	if res.EvalNanos > 0 {
		r.TokensPerSec = float64(res.EvalTokens) / (float64(res.EvalNanos) / 1e9)
	}
	for ref, s := range book.Items {
		r.Sources[ref] = s.Service + ":" + s.ObjectID
	}
	r.Answer, r.Cited, r.Invalid = CleanCitations(res.Text, func(ref string) bool { return book.Items[ref] != nil })
	if err != nil {
		r.Error = err.Error()
	}
	r.Failures = score(c, f, data, book, r, h.Lang)
	r.Passed = len(r.Failures) == 0
	return r
}

func containsFold(hay, needle string) bool {
	return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}

func score(c castCase, f *castFixture, data castData, book *sourceBook, r castResult, lang *languagePack) []string {
	var fails []string
	add := func(format string, args ...any) { fails = append(fails, fmt.Sprintf(format, args...)) }
	ch := c.Checks
	answer := r.Answer
	if r.Error != "" {
		add("error")
	}
	if r.Truncated {
		add("truncated")
	}
	if r.Invalid > 0 {
		add("invalid markers")
	}
	if cjk.MatchString(r.Raw) {
		add("foreign script")
	}
	if lang.claimsChange(answer) {
		add("claims executed change")
	}
	for i, want := range ch.Calls {
		if i >= len(r.Calls) {
			add("missing call %s", want.Name)
			break
		}
		got := r.Calls[i]
		if got.Name != want.Name {
			add("call %d is %s, want %s", i+1, got.Name, want.Name)
			continue
		}
		var args map[string]any
		if json.Unmarshal(got.Args, &args) != nil {
			add("call %d arguments invalid", i+1)
			continue
		}
		// The primary expectation or any documented equivalent must match.
		var best []string
		for k, expectation := range append([]map[string]any{want.Args}, want.Alternatives...) {
			problems := checkArgs(want.Name, expectation, want.AllowExtra, args, data, book)
			if k == 0 || len(problems) < len(best) {
				best = problems
			}
			if len(problems) == 0 {
				break
			}
		}
		fails = append(fails, best...)
	}
	if ch.NoCalls && len(r.Calls) > 0 {
		add("unexpected tool call %s", r.Calls[0].Name)
	}
	for _, call := range r.Calls {
		for _, forbidden := range ch.ForbidCalls {
			if call.Name == forbidden {
				add("forbidden call %s", forbidden)
			}
		}
		for _, q := range ch.ForbidQueries {
			if containsFold(string(call.Args), q) {
				add("forbidden query %s", q)
			}
		}
	}
	for _, s := range ch.AnswerAll {
		if !containsFold(answer, s) {
			add("answer lacks %q", s)
		}
	}
	for _, group := range [][]string{ch.AnswerAny, ch.AnswerAny2} {
		if len(group) == 0 {
			continue
		}
		ok := false
		for _, s := range group {
			ok = ok || containsFold(answer, s)
		}
		if !ok {
			add("answer lacks any of %v", group)
		}
	}
	for _, s := range ch.AnswerNone {
		if containsFold(answer, s) {
			add("answer contains %q", s)
		}
	}
	returned := map[string]bool{}
	for _, s := range book.Items {
		if s.Service == "media" {
			returned[s.Title] = true
		}
	}
	for _, t := range ch.TitlesAll {
		if !containsFold(answer, t) {
			add("title missing %s", t)
		}
	}
	for _, t := range ch.TitlesNone {
		if containsFold(answer, t) {
			add("title should not appear %s", t)
		}
	}
	if ch.TitlesOnly {
		for _, m := range f.Data.Movies {
			if containsFold(answer, m.Title) && !returned[m.Title] {
				add("title not from results %s", m.Title)
			}
		}
	}
	for _, key := range ch.Cite {
		ref := data.objectRef(book, key)
		cited := false
		for _, c := range r.Cited {
			cited = cited || c == ref
		}
		if ref == "" || !cited {
			add("missing citation %s", key)
		}
	}
	if ch.NoCitations && len(r.Cited) > 0 {
		add("unexpected citation")
	}
	if (ch.German || ch.Language) && lang.wrongLanguage(answer, book.titles()...) {
		add("answer not in %s", lang.Name)
	}
	if ch.Question && !strings.Contains(answer, "?") {
		add("no follow-up question")
	}
	if strings.TrimSpace(answer) == "" && len(fails) == 0 {
		add("empty answer")
	}
	return fails
}

// canonicalArgs compares filter meaning, not spelling: the v4 movie contract
// offers equivalent forms (minutes or seconds, inclusive or exclusive bound,
// watched or unwatched). Runtimes are whole seconds.
func canonicalArgs(name string, in map[string]any) map[string]any {
	if name != "search_movies" {
		return in
	}
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	num := func(k string) (float64, bool) {
		n, ok := out[k].(float64)
		delete(out, k)
		return n, ok
	}
	if b, ok := out["unwatched"].(bool); ok {
		delete(out, "unwatched")
		if _, set := out["watched"]; !set {
			out["watched"] = !b
		}
	}
	for _, alias := range [][2]string{{"runtime_under_seconds", "runtime_below_seconds"}, {"runtime_under_minutes", "runtime_below_minutes"},
		{"runtime_at_most_seconds", "runtime_max_seconds"}, {"runtime_at_most_minutes", "runtime_max_minutes"}} {
		if n, ok := num(alias[0]); ok {
			out[alias[1]] = n
		}
	}
	if n, ok := num("runtime_below_minutes"); ok {
		out["runtime_below_seconds"] = n * 60
	}
	if n, ok := num("runtime_max_minutes"); ok {
		out["runtime_max_seconds"] = n * 60
	}
	if n, ok := num("runtime_max_seconds"); ok {
		out["runtime_below_seconds"] = n + 1
	}
	return out
}

func checkArgs(name string, expectedArgs map[string]any, allowExtra []string, args map[string]any, data castData, book *sourceBook) []string {
	expectedArgs, args = canonicalArgs(name, expectedArgs), canonicalArgs(name, args)
	var fails []string
	add := func(format string, a ...any) { fails = append(fails, fmt.Sprintf(format, a...)) }
	for key, expected := range expectedArgs {
		value, ok := args[key]
		if !ok {
			add("%s missing %s", name, key)
			continue
		}
		switch e := expected.(type) {
		case string:
			if key == "source" {
				if ref := data.objectRef(book, e); ref == "" || strings.Trim(fmt.Sprint(value), "[]") != ref {
					add("%s source %v, want %s", name, value, ref)
				}
			} else if s, ok := value.(string); !ok || !containsFold(s, e) {
				add("%s %s=%v", name, key, value)
			}
		case float64:
			if n, ok := value.(float64); !ok || math.Abs(n-e) > 0.001 {
				add("%s %s=%v want %v", name, key, value, e)
			}
		case bool:
			if b, ok := value.(bool); !ok || b != e {
				add("%s %s=%v want %v", name, key, value, e)
			}
		}
	}
	for key, value := range args {
		if _, expected := expectedArgs[key]; expected {
			continue
		}
		allowed := false
		for _, a := range allowExtra {
			allowed = allowed || a == key
		}
		if !allowed && value != nil && value != "" {
			add("%s unexpected %s=%v", name, key, value)
		}
	}
	return fails
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	idx := int(math.Ceil(p*float64(len(values)))) - 1
	return values[max(0, min(idx, len(values)-1))]
}

func summarize(all []castResult) map[string]any {
	out := map[string]any{}
	byModel := map[string][]castResult{}
	for _, r := range all {
		byModel[r.Model] = append(byModel[r.Model], r)
	}
	for model, rs := range byModel {
		cats := map[string][2]int{}
		var first, total, tps []float64
		invalid, errorsN := 0, 0
		corrections := map[string]int{}
		for i, r := range rs {
			c := cats[r.Category]
			c[1]++
			if r.Passed {
				c[0]++
			}
			cats[r.Category] = c
			invalid += r.Invalid
			for _, c := range r.Corrections {
				corrections[c]++
			}
			if r.Error != "" {
				errorsN++
			}
			if i == 0 {
				continue // cold load
			}
			first = append(first, r.FirstSeconds)
			if r.Category == "tools" {
				total = append(total, r.Seconds)
			}
			if r.TokensPerSec > 0 {
				tps = append(tps, r.TokensPerSec)
			}
		}
		categories := map[string]string{}
		for k, v := range cats {
			categories[k] = fmt.Sprintf("%d/%d", v[0], v[1])
		}
		out[model] = map[string]any{"categories": categories, "invalidMarkers": invalid, "errors": errorsN, "interventions": corrections,
			"p95FirstSeconds": percentile(first, 0.95), "p95ToolAnswerSeconds": percentile(total, 0.95), "medianTokensPerSecond": percentile(tps, 0.5)}
	}
	return out
}
