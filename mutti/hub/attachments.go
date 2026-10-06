// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Chat attachments stay private to one conversation. They are never imported
// into Paperless automatically; text is extracted locally without network.
const attachmentLimit = 20 << 20

var attachmentTypes = map[string]bool{"text/plain": true, "text/markdown": true, "text/csv": true, "application/pdf": true}

func findAttachment(c *Conversation, id string) *Attachment {
	for _, a := range c.Attachments {
		if a.ID == id {
			return a
		}
	}
	return nil
}

func (a *AI) attachmentDir(c *Conversation) string {
	return filepath.Join(a.dir, "attachments", c.Owner, c.ID)
}

func (a *AI) postAttachment(w http.ResponseWriter, r *http.Request, id Identity) error {
	if err := a.access(id); err != nil {
		return err
	}
	mime := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
	name, _ := url.PathUnescape(r.Header.Get("X-Filename"))
	name = filepath.Base(strings.TrimSpace(name))
	if !attachmentTypes[mime] || name == "" || name == "." || len(name) > 200 || !utf8.ValidString(name) {
		return apiErr(415, "unsupported", "Unterstützt werden Text- und PDF-Dateien bis 20 MB.")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, attachmentLimit))
	if err != nil {
		return apiErr(413, "too_large", "Die Datei ist größer als 20 MB.")
	}
	cid := r.PathValue("id")
	unlock := a.lock(cid)
	defer unlock()
	c, keys, err := a.load(id.UserID, cid)
	if err != nil {
		return err
	}
	// A running answer works on a copy of the sources; a marker issued now
	// would be lost or reused when that answer is stored.
	for _, run := range c.Runs {
		if run.State == "queued" || run.State == "running" {
			return apiErr(409, "busy", "Bitte warte, bis die laufende Antwort fertig ist, oder brich sie ab.")
		}
	}
	if len(c.Attachments) >= 20 {
		return apiErr(409, "limit", "Dieses Gespräch hat bereits 20 Anhänge.")
	}
	att := &Attachment{ID: randomID(), Name: name, Mime: mime, Size: int64(len(body)), Created: time.Now().UTC()}
	text := ""
	if mime == "application/pdf" {
		text = extractPDFText(body)
	} else if utf8.Valid(body) {
		text = string(body)
	}
	text = strings.TrimSpace(text)
	switch {
	case text == "" && mime == "application/pdf":
		att.Extracted = "no_text"
	case text == "":
		att.Extracted = "failed"
	default:
		att.Extracted = "ok"
	}
	dir := a.attachmentDir(c)
	if err = writePrivate(filepath.Join(dir, att.ID), body); err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(dir, att.ID+".txt"), []byte(text)); err != nil {
		return err
	}
	s := c.Sources.add(Source{Service: "attachment", Kind: "attachment", ObjectID: att.ID, Title: name})
	att.Ref = s.Ref
	c.Attachments = append(c.Attachments, att)
	if err = a.save(c, keys); err != nil {
		return err
	}
	writeJSON(w, 201, att)
	return nil
}

func (a *AI) attachmentText(c *Conversation, at *Attachment) string {
	b, err := os.ReadFile(filepath.Join(a.attachmentDir(c), at.ID+".txt"))
	if err != nil || len(b) == 0 {
		return "(Kein lesbarer Text. Für gescannte Dokumente bitte den Import ins Dokumentenarchiv mit Texterkennung verwenden.)"
	}
	text := string(b)
	if len(text) > 8000 {
		text = text[:8000] + " …"
	}
	return text
}

func (a *AI) getAttachment(w http.ResponseWriter, r *http.Request, id Identity) error {
	if err := a.access(id); err != nil {
		return err
	}
	c, _, err := a.load(id.UserID, r.PathValue("id"))
	if err != nil {
		return err
	}
	at := findAttachment(c, r.PathValue("attachment"))
	if at == nil {
		return errNotFound
	}
	f, err := os.Open(filepath.Join(a.attachmentDir(c), at.ID))
	if err != nil {
		return errNotFound
	}
	defer f.Close()
	w.Header().Set("Content-Type", at.Mime)
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+urlPathEscape(at.Name))
	http.ServeContent(w, r, "", at.Created, f)
	return nil
}

var (
	pdfStream = regexp.MustCompile(`(?s)<<(.*?)>>\s*stream\r?\n`)
	pdfHex    = regexp.MustCompile(`^[0-9A-Fa-f\s]*$`)
)

// extractPDFText reads the text layer of simple PDFs (uncompressed or Flate
// content streams, standard Latin encodings). Scans and CID-font PDFs yield no
// text and are reported as such instead of guessing.
func extractPDFText(data []byte) string {
	var out strings.Builder
	for _, loc := range pdfStream.FindAllSubmatchIndex(data, -1) {
		dict := string(data[loc[2]:loc[3]])
		start := loc[1]
		end := bytes.Index(data[start:], []byte("endstream"))
		if end < 0 {
			continue
		}
		raw := data[start : start+end]
		if strings.Contains(dict, "/Image") || strings.Contains(dict, "/XObject") {
			continue
		}
		if strings.Contains(dict, "/FlateDecode") {
			zr, err := zlib.NewReader(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			inflated, err := io.ReadAll(io.LimitReader(zr, 8<<20))
			zr.Close()
			if err != nil && len(inflated) == 0 {
				continue
			}
			raw = inflated
		} else if strings.Contains(dict, "/Filter") {
			continue
		}
		if !bytes.Contains(raw, []byte("BT")) {
			continue
		}
		parseContent(raw, &out)
		if out.Len() > 400_000 {
			break
		}
	}
	return strings.TrimSpace(out.String())
}

func parseContent(b []byte, out *strings.Builder) {
	var pending []string
	flush := func() {
		for _, s := range pending {
			out.WriteString(s)
		}
		pending = pending[:0]
	}
	i := 0
	for i < len(b) {
		c := b[i]
		switch {
		case c == '(':
			s, n := readLiteral(b[i:])
			pending = append(pending, s)
			i += n
		case c == '<' && i+1 < len(b) && b[i+1] != '<':
			j := bytes.IndexByte(b[i:], '>')
			if j < 0 {
				return
			}
			h := string(b[i+1 : i+j])
			if pdfHex.MatchString(h) {
				h = strings.Join(strings.Fields(h), "")
				if len(h)%2 == 1 {
					h += "0"
				}
				if decoded, err := hex.DecodeString(h); err == nil && printable(decoded) {
					pending = append(pending, latin1(decoded))
				}
			}
			i += j + 1
		case c == '[' || c == ']':
			i++
		case isOperatorStart(c):
			j := i
			for j < len(b) && isOperatorChar(b[j]) {
				j++
			}
			op := string(b[i:j])
			switch op {
			case "Tj", "TJ":
				flush()
			case "'", "\"", "T*", "Td", "TD", "ET":
				flush()
				if !strings.HasSuffix(out.String(), "\n") && out.Len() > 0 {
					out.WriteString("\n")
				}
			default:
				pending = pending[:0]
			}
			if j == i {
				j++
			}
			i = j
		default:
			i++
		}
	}
}

func isOperatorStart(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '\'' || c == '"' || c == '*'
}

func isOperatorChar(c byte) bool { return isOperatorStart(c) || c == '*' }

func readLiteral(b []byte) (string, int) {
	depth := 0
	var buf []byte
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '\\' && i+1 < len(b):
			i++
			switch e := b[i]; e {
			case 'n':
				buf = append(buf, '\n')
			case 'r', 't', 'b', 'f':
				buf = append(buf, ' ')
			case '(', ')', '\\':
				buf = append(buf, e)
			default:
				if e >= '0' && e <= '7' {
					v, k := 0, 0
					for k < 3 && i < len(b) && b[i] >= '0' && b[i] <= '7' {
						v = v*8 + int(b[i]-'0')
						i++
						k++
					}
					i--
					buf = append(buf, byte(v))
				}
			}
		case c == '(':
			depth++
			if depth > 1 {
				buf = append(buf, c)
			}
		case c == ')':
			depth--
			if depth == 0 {
				return latin1(buf), i + 1
			}
			buf = append(buf, c)
		default:
			buf = append(buf, c)
		}
	}
	return latin1(buf), len(b)
}

func printable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 && c != '\n' && c != '\t' {
			return false
		}
	}
	return true
}

func latin1(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}
