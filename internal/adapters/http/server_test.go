//go:build darwin

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"movement.local/movement/internal/app"
	store "movement.local/movement/internal/store/text"
)

func TestAuthenticationAndSharedRepository(t *testing.T) {
	ctx := context.Background()
	repo := store.New(t.TempDir())
	if _, e := repo.Initialize(ctx, "UTC"); e != nil {
		t.Fatal(e)
	}
	token, e := repo.APIToken(ctx)
	if e != nil {
		t.Fatal(e)
	}
	svc := app.Service{Repo: repo}
	h, e := New(svc, Options{Host: "127.0.0.1:8484", Token: token, Timezone: "UTC"})
	if e != nil {
		t.Fatal(e)
	}
	request := func(method, path, body, auth, origin, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:8484"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	assertStatus := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d want %d: %s", w.Code, status, w.Body.String())
		}
	}
	assertStatus(request("GET", "/api/v1/tasks", "", "", "", "", nil), 401)
	assertStatus(request("GET", "/api/v1/tasks", "", token, "http://evil.example", "", nil), 403)
	badHost := httptest.NewRequest("GET", "http://evil.example/api/v1/tasks", nil)
	badHost.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, badHost)
	assertStatus(w, 403)
	payload := `{"operation_id":"http-create","actor":"http-test","data":{"title":"HTTP task","status":"ready"}}`
	w = request("POST", "/api/v1/tasks", payload, token, "", "", nil)
	assertStatus(w, 200)
	var receipt app.Receipt
	if e = json.Unmarshal(w.Body.Bytes(), &receipt); e != nil {
		t.Fatal(e)
	}
	if b, e := svc.Get(ctx, receipt.TaskID); e != nil || b.Task.Title != "HTTP task" {
		t.Fatalf("shared read: %v %+v", e, b)
	}
	retried := request("POST", "/api/v1/tasks", payload, token, "", "", nil)
	assertStatus(retried, 200)
	if retried.Body.String() != w.Body.String() {
		t.Fatal("retry receipt changed")
	}
	assertStatus(request("GET", "/api/v1/tasks?limit=no", "", token, "", "", nil), 400)
	assertStatus(request("GET", "/api/v1/tasks?from=2026-10-24&to=2026-10-27", "", token, "", "", nil), 200)
	assertStatus(request("GET", "/api/v1/tasks?from=2026-10-24&to=2026-10-27&from=2026-10-25", "", token, "", "", nil), 400)
	assertStatus(request("GET", "/api/v1/tasks?unknown=x", "", token, "", "", nil), 400)
	assertStatus(request("GET", "/api/v1/tasks?include_overdue=true&statuses=ready", "", token, "", "", nil), 200)
	w = request("POST", "/api/v1/session", "", token, "http://127.0.0.1:8484", "", nil)
	assertStatus(w, 200)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	cookie := cookies[0]
	var sessionData struct {
		CSRF string `json:"csrf_token"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &sessionData); e != nil {
		t.Fatal(e)
	}
	assertStatus(request("GET", "/api/v1/session", "", "", "", "", cookie), 200)
	edit := `{"operation_id":"http-edit","actor":"browser","expected_revision":1,"data":{"fields":{"title":"Browser edit"}}}`
	path := "/api/v1/tasks/" + receipt.TaskID
	assertStatus(request("PATCH", path, edit, "", "http://127.0.0.1:8484", "", cookie), 403)
	assertStatus(request("PATCH", path, edit, "", "", sessionData.CSRF, cookie), 403)
	assertStatus(request("PATCH", path, edit, "", "http://127.0.0.1:8484", sessionData.CSRF, cookie), 200)
	assertStatus(request("PATCH", path, strings.Replace(edit, "http-edit", "stale", 1), token, "", "", nil), 409)
	for _, suffix := range []string{"", "/context", "/history?offset=0&limit=1"} {
		assertStatus(request("GET", path+suffix, "", "", "", "", cookie), 200)
	}
	assertStatus(request("POST", "/api/v1/tasks", `{"actor":"a","actor":"b"}`, token, "", "", nil), 400)
	assertStatus(request("DELETE", "/api/v1/session", "", "", "http://127.0.0.1:8484", sessionData.CSRF, cookie), 200)
	assertStatus(request("GET", path, "", "", "", "", cookie), 401)
	// Expiry is checked server-side, independently of browser cookie lifetime.
	h.sessions[cookie.Value] = session{CSRF: "expired", Expires: time.Now().Add(-time.Second)}
	assertStatus(request("GET", path, "", "", "", "", cookie), 401)
}
func TestDevOriginIsExplicit(t *testing.T) {
	for _, origin := range []string{"https://evil.example", "http://localhost:5173/path", "http://localhost", "http://localhost:5173?x=1"} {
		if _, e := New(app.Service{}, Options{Host: "127.0.0.1:8484", Token: strings.Repeat("a", 64), DevOrigin: origin}); e == nil {
			t.Fatalf("accepted %s", origin)
		}
	}
	if _, e := New(app.Service{}, Options{Host: "0.0.0.0:8484", Token: strings.Repeat("a", 64)}); e == nil {
		t.Fatal("accepted public binding")
	}
	h, e := New(app.Service{}, Options{Host: "127.0.0.1:8484", Token: strings.Repeat("a", 64), DevOrigin: "http://localhost:5173"})
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "http://localhost:5173/api/v1/session", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unexpected CORS allowance")
	}
}
