package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedNavigationAndMissingAssets(t *testing.T) {
	for _, tc := range []struct {
		path   string
		status int
	}{{"/", 200}, {"/assets/missing.js", 404}, {"/api/v1/missing", 404}, {"/assets/", 404}, {"/missing", 404}} {
		w := httptest.NewRecorder()
		Handler().ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if tc.path == "/" && (!strings.Contains(w.Body.String(), "/assets/") || strings.Contains(w.Body.String(), "https://")) {
			t.Fatal("entry page missing local assets or contains remote URL")
		}
	}
}
