// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminBrandAssetsStayInsideOriginBoundary(t *testing.T) {
	handler := (&Server{}).Admin("http://127.0.0.1:18595")
	for _, path := range []string{"/", "/sora.woff2", "/sora-semibold.woff2", "/sora-bold.woff2", "/mark-light.svg", "/wordmark-light.svg"} {
		for _, foreign := range []string{"", "host", "origin"} {
			req := httptest.NewRequest("GET", "http://127.0.0.1:18595"+path, nil)
			if foreign == "host" {
				req.Host = "attacker.example"
			}
			if foreign == "origin" {
				req.Header.Set("Origin", "https://attacker.example")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if foreign != "" {
				if response.Code != 403 {
					t.Fatalf("%s: foreign %s accepted", path, foreign)
				}
				continue
			}
			if response.Code != 200 || response.Body.Len() == 0 {
				t.Fatalf("%s: resource unavailable", path)
			}
			csp := response.Header().Get("Content-Security-Policy")
			if !strings.Contains(csp, "font-src 'self';") || !strings.Contains(csp, "default-src 'none';") {
				t.Fatalf("%s: fonts blocked or default policy changed", path)
			}
			if strings.HasSuffix(path, ".woff2") && response.Header().Get("Content-Type") != "font/woff2" {
				t.Fatal("wrong font type")
			}
			if strings.HasSuffix(path, ".svg") && response.Header().Get("Content-Type") != "image/svg+xml" {
				t.Fatal("wrong logo type")
			}
		}
	}
}
