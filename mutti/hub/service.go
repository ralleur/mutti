// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// serviceError hides upstream bodies; clients receive stable codes only.
func serviceError(status int) error {
	switch {
	case status == 401 || status == 403:
		return apiErr(403, "service_denied", "Der Dienst hat den Zugriff für dieses Profil abgelehnt. Bitte die Kontozuordnung in Mutti prüfen.")
	case status == 404 || status == 400:
		return errNotFound
	case status == 413:
		return apiErr(413, "too_large", "Die Datei ist zu groß.")
	case status == 507:
		return apiErr(507, "storage_full", "Auf dem Server ist nicht genug Speicher frei.")
	default:
		return apiErr(502, "unavailable", "Der Dienst ist gerade nicht erreichbar.")
	}
}

func serviceCall(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		return apiErr(502, "unavailable", "Der Dienst ist gerade nicht erreichbar.")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return serviceError(res.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return nil
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(out); err != nil {
		return apiErr(502, "unavailable", "Unerwartete Antwort des Dienstes.")
	}
	return nil
}

// relay streams a binary upstream response. Only content headers pass; no
// cookies, redirects or upstream URLs reach the device. Range is honoured so
// videos and large originals can be played and resumed.
func relay(w http.ResponseWriter, r *http.Request, client *http.Client, url string, headers map[string]string, filename string) error {
	req, err := http.NewRequestWithContext(r.Context(), "GET", url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if rg := r.Header.Get("Range"); rg != "" && len(rg) < 128 {
		req.Header.Set("Range", rg)
	}
	res, err := client.Do(req)
	if err != nil {
		return apiErr(502, "unavailable", "Der Dienst ist gerade nicht erreichbar.")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		return serviceError(res.StatusCode)
	}
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := res.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	if filename != "" {
		w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+urlPathEscape(filename))
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
	return nil
}

func urlPathEscape(s string) string {
	var b bytes.Buffer
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func queryInt(r *http.Request, name string, def, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
