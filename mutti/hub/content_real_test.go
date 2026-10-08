// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Federated search against real Immich and Paperless from module-testenv.py.
// Opt-in with MUTTI_MODULE_TESTENV like the other real-module tests.
func TestRealContentSearch(t *testing.T) {
	env := loadTestenv(t)
	base := newTestEnv(t, "")
	e := &contentEnv{testEnv: base}
	e.mustAdmin("/admin/service/photos", map[string]string{"url": env.PhotosURL})
	e.mustAdmin("/admin/service/documents", map[string]string{"url": env.DocumentsURL})
	for user, name := range map[string]string{userA: "alpha", userB: "beta"} {
		e.mustAdmin("/admin/link/photos", map[string]string{"userId": user, "account": env.Accounts.Photos[name].Email, "password": env.Accounts.Photos[name].Password})
		e.mustAdmin("/admin/link/documents", map[string]string{"userId": user, "account": env.Accounts.Documents[name].Username, "password": env.Accounts.Documents[name].Password})
		for _, module := range []string{"photos", "documents"} {
			e.mustAdmin("/admin/grants", map[string]any{"module": module, "userId": user, "allowed": true})
		}
	}
	e.mustAdmin("/admin/enable/photos", map[string]bool{"enabled": true})
	e.mustAdmin("/admin/enable/documents", map[string]bool{"enabled": true})
	e.hub.refreshHealth(context.Background())

	a, code, rawA := e.search("token-a", "size=50")
	if code != 200 || a.Completeness != "complete" {
		t.Fatalf("%d %s", code, rawA)
	}
	b, _, rawB := e.search("token-b", "size=50")
	for _, id := range []string{env.PhotosFixture["private_b"], fmt.Sprintf(`"id":%d,`, env.DocumentsFixture["private_b"]), "Arztrechnung", "999,99"} {
		if strings.Contains(rawA, id) {
			t.Fatalf("A sees B's %s", id)
		}
	}
	for _, id := range []string{env.PhotosFixture["see"], fmt.Sprintf(`"id":%d,`, env.DocumentsFixture["invoice"])} {
		if !strings.Contains(rawA, id) || strings.Contains(rawB, id) {
			t.Fatalf("own/foreign mismatch for %s", id)
		}
	}
	if !strings.Contains(rawB, env.PhotosFixture["private_b"]) {
		t.Fatal("B misses own photo")
	}
	for _, it := range append(a.Items, b.Items...) {
		if it.Area != AreaMedia && it.Ref.Revision == nil {
			t.Fatalf("no revision from real backend: %+v", it)
		}
	}
	// Full-text, date and kind filters through the real services.
	p, _, raw := e.search("token-a", "q=Rechnung&kinds=document")
	if len(p.Items) == 0 || p.Items[0].Document.ID != env.DocumentsFixture["invoice"] || strings.Contains(raw, "<span") {
		t.Fatalf("document search %s", raw)
	}
	if p, _, _ := e.search("token-a", "kinds=document&from=2100-01-01"); len(p.Items) != 0 || p.Completeness != "complete" {
		t.Fatalf("date filter %v", titles(p))
	}
	if p, _, _ := e.search("token-a", "kinds=video"); len(p.Items) != 1 || p.Items[0].Photo.ID != env.PhotosFixture["clip"] {
		t.Fatalf("video filter %v", titles(p))
	}
	// Opening re-authorizes against the real backend: B cannot use A's ID.
	p, _, _ = e.search("token-a", "q=Rechnung&kinds=document")
	invoice := p.Items
	if code := e.do("GET", "/content/items/"+invoice[0].Ref.ContentID, "token-b", nil, nil); code != 404 {
		t.Fatalf("B opens A's document %d", code)
	}
	res, body := e.raw("GET", "/content/items/"+invoice[0].Ref.ContentID+"/thumbnail", "token-a", nil, "", nil)
	if res.StatusCode != 200 || len(body) == 0 {
		t.Fatalf("thumbnail %d", res.StatusCode)
	}
}
