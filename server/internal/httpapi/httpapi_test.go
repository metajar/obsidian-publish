package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"obsidian-publish/server/internal/assets"
	"obsidian-publish/server/internal/ratelimit"
	"obsidian-publish/server/internal/session"
	"obsidian-publish/server/internal/store"
)

const testToken = "test-api-token-0123456789abcdef"

type testServer struct {
	e      *echo.Echo
	s      *Server
	dbPath string
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	assetStore, err := assets.Open(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatalf("assets.Open: %v", err)
	}
	sessions, err := session.NewManager([]byte("test-secret-0123456789abcdef"), time.Hour)
	if err != nil {
		t.Fatalf("session.NewManager: %v", err)
	}
	e, srv := New(Deps{
		Store:    st,
		Token:    testToken,
		Sessions: sessions,
		Limiter:  ratelimit.New(5, time.Minute),
		BaseURL:  "https://notes.example.com",
		Assets:   assetStore,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return &testServer{e: e, s: srv, dbPath: dbPath}
}

// do performs a request against the test server. api decides whether the
// bearer token is attached; ip overrides the client RemoteAddr.
func (ts *testServer) do(t *testing.T, method, target, body string, api bool, ip string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if api {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+testToken)
	}
	if ip != "" {
		req.RemoteAddr = ip + ":31337"
	}
	rec := httptest.NewRecorder()
	ts.e.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode JSON %q: %v", rec.Body.String(), err)
	}
	return m
}

// --- bearer token middleware ---

func TestBearerAuth(t *testing.T) {
	ts := newTestServer(t)
	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"not bearer", "Token " + testToken, http.StatusUnauthorized},
		{"basic scheme", "Basic " + testToken, http.StatusUnauthorized},
		{"missing value", "Bearer", http.StatusUnauthorized},
		{"empty value", "Bearer ", http.StatusUnauthorized},
		{"wrong token", "Bearer wrong-token-entirely-wrong", http.StatusUnauthorized},
		{"prefix of token", "Bearer " + testToken[:len(testToken)-1], http.StatusUnauthorized},
		{"valid", "Bearer " + testToken, http.StatusOK},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/pages", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		rec := httptest.NewRecorder()
		ts.e.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (body %q)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}

	// Error body is JSON on the API plane and never echoes the token.
	if !strings.Contains(rec0(ts, t).Body.String(), "error") {
		t.Error("401 body missing error field")
	}
}

func rec0(ts *testServer, t *testing.T) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/pages", nil)
	rec := httptest.NewRecorder()
	ts.e.ServeHTTP(rec, req)
	return rec
}

// Public plane must never require the token.
func TestPublicPlaneDoesNotRequireToken(t *testing.T) {
	ts := newTestServer(t)
	rec := ts.do(t, http.MethodGet, "/no-such-page", "", false, "9.9.9.9")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /no-such-page = %d, want 404 without any token", rec.Code)
	}
}

// --- full lifecycle ---

func TestPageLifecycle(t *testing.T) {
	ts := newTestServer(t)

	// Route available before publish.
	rec := ts.do(t, http.MethodGet, "/api/routes/trip-plan/available", "", true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("available check = %d: %s", rec.Code, rec.Body.String())
	}
	if got := decodeJSON(t, rec)["available"]; got != true {
		t.Errorf("available = %v, want true", got)
	}

	// Invalid route reports unavailable (not an error).
	rec = ts.do(t, http.MethodGet, "/api/routes/Bad%20Route/available", "", true, "")
	if rec.Code != http.StatusOK || decodeJSON(t, rec)["available"] != false {
		t.Errorf("invalid route available check = %d %s", rec.Code, rec.Body.String())
	}

	// Publish.
	pw := "hunter2"
	body := fmt.Sprintf(`{"route":"trip-plan","title":"Trip Plan","markdown":"# Trip\n\n| day | city |\n| - | - |\n| 1 | Rome |","password":%q}`, pw)
	rec = ts.do(t, http.MethodPost, "/api/pages", body, true, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d: %s", rec.Code, rec.Body.String())
	}
	created := decodeJSON(t, rec)
	if created["route"] != "trip-plan" || created["password_protected"] != true {
		t.Errorf("publish response = %v", created)
	}
	if created["url"] != "https://notes.example.com/trip-plan" {
		t.Errorf("url = %v", created["url"])
	}
	if _, leaked := created["password_hash"]; leaked {
		t.Error("password_hash leaked in publish response")
	}

	// Route now taken.
	rec = ts.do(t, http.MethodGet, "/api/routes/trip-plan/available", "", true, "")
	if decodeJSON(t, rec)["available"] != false {
		t.Error("available after publish = true, want false")
	}

	// Duplicate publish conflicts.
	rec = ts.do(t, http.MethodPost, "/api/pages", body, true, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate publish = %d, want 409", rec.Code)
	}

	// Invalid route rejected with 400.
	rec = ts.do(t, http.MethodPost, "/api/pages", `{"route":"Bad Route","markdown":"x"}`, true, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid-route publish = %d, want 400", rec.Code)
	}

	// List.
	rec = ts.do(t, http.MethodGet, "/api/pages", "", true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d", rec.Code)
	}
	var pages []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &pages); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if len(pages) != 1 || pages[0]["route"] != "trip-plan" || pages[0]["password_protected"] != true {
		t.Errorf("list = %v", pages)
	}
	if strings.Contains(rec.Body.String(), "password_hash") || strings.Contains(rec.Body.String(), pw) {
		t.Error("list leaked password_hash or plaintext password")
	}

	// Update: change markdown and remove password (explicit null).
	rec = ts.do(t, http.MethodPut, "/api/pages/trip-plan", `{"markdown":"# Trip v2\n\nupdated","password":null}`, true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}
	updated := decodeJSON(t, rec)
	if updated["password_protected"] != false {
		t.Errorf("password_protected after null = %v, want false", updated["password_protected"])
	}

	// Public GET now serves the content directly.
	rec = ts.do(t, http.MethodGet, "/trip-plan", "", false, "8.8.8.8")
	if rec.Code != http.StatusOK {
		t.Fatalf("public GET after unprotect = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Trip v2") {
		t.Errorf("public GET body missing updated content:\n%s", rec.Body.String())
	}

	// Update: set a new password with password omitted-then-set semantics.
	rec = ts.do(t, http.MethodPut, "/api/pages/trip-plan", `{"password":"new-pass"}`, true, "")
	if rec.Code != http.StatusOK || decodeJSON(t, rec)["password_protected"] != true {
		t.Fatalf("re-protect update = %d: %s", rec.Code, rec.Body.String())
	}

	// Update unknown route 404s.
	rec = ts.do(t, http.MethodPut, "/api/pages/nope", `{"markdown":"x"}`, true, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("update unknown route = %d, want 404", rec.Code)
	}

	// Unpublish: 200 with a JSON body (not 204) so Obsidian's requestUrl
	// can parse the response — an empty body makes it throw a JSON EOF error.
	rec = ts.do(t, http.MethodDelete, "/api/pages/trip-plan", "", true, "")
	if rec.Code != http.StatusOK || decodeJSON(t, rec)["deleted"] != "trip-plan" {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body.String())
	}
	rec = ts.do(t, http.MethodGet, "/trip-plan", "", false, "8.8.8.8")
	if rec.Code != http.StatusNotFound {
		t.Errorf("public GET after delete = %d, want 404", rec.Code)
	}
	rec = ts.do(t, http.MethodDelete, "/api/pages/trip-plan", "", true, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
	rec = ts.do(t, http.MethodGet, "/api/routes/trip-plan/available", "", true, "")
	if decodeJSON(t, rec)["available"] != true {
		t.Error("available after delete = false, want true")
	}
}

// --- password-only PUT (settings-row password rotation) ---

// TestUpdatePagePasswordOnly proves the contract the plugin's settings table
// relies on: PUT /api/pages/{route} with a body of only {"password": "..."}
// sets/replaces the password and leaves markdown, title, and the per-page
// theme override untouched — on both protected and previously-public pages.
func TestUpdatePagePasswordOnly(t *testing.T) {
	ts := newTestServer(t)

	// Protected page with a title, content, and a theme override.
	body := `{"route":"goals-alice","title":"Goals","markdown":"# Goals for Alice","password":"old-pass","theme_css":"body { background: papayawhip; }"}`
	rec := ts.do(t, http.MethodPost, "/api/pages", body, true, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d: %s", rec.Code, rec.Body.String())
	}

	// Password-only PUT: replace.
	rec = ts.do(t, http.MethodPut, "/api/pages/goals-alice", `{"password":"new-pass"}`, true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("password-only update = %d: %s", rec.Code, rec.Body.String())
	}
	updated := decodeJSON(t, rec)
	if updated["route"] != "goals-alice" || updated["title"] != "Goals" || updated["password_protected"] != true {
		t.Errorf("password-only update response = %v", updated)
	}

	// The old password stops working; the new one unlocks the page.
	if rec := ts.doForm(t, http.MethodPost, "/goals-alice/auth", "password=old-pass", "3.3.3.1"); rec.Code != http.StatusUnauthorized {
		t.Errorf("old password = %d, want 401", rec.Code)
	}
	rec = ts.doForm(t, http.MethodPost, "/goals-alice/auth", "password=new-pass", "3.3.3.2")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("new password = %d, want 303", rec.Code)
	}
	ck := rec.Result().Cookies()[0]

	// Content, title, and theme override all survive the password-only PUT.
	req := httptest.NewRequest(http.MethodGet, "/goals-alice", nil)
	req.Header.Set("Cookie", session.CookieName+"="+ck.Value)
	authed := httptest.NewRecorder()
	ts.e.ServeHTTP(authed, req)
	if authed.Code != http.StatusOK {
		t.Fatalf("GET with session = %d", authed.Code)
	}
	page := authed.Body.String()
	if !strings.Contains(page, "Goals for Alice") {
		t.Error("markdown content not preserved by password-only update")
	}
	if !strings.Contains(page, "<title>Goals</title>") {
		t.Error("title not preserved by password-only update")
	}
	if !strings.Contains(page, "papayawhip") {
		t.Error("per-page theme override not preserved by password-only update")
	}
	if strings.Contains(page, "old-pass") || strings.Contains(page, "new-pass") {
		t.Error("plaintext password leaked into served page")
	}

	// A previously-public page can be protected the same way.
	rec = ts.do(t, http.MethodPost, "/api/pages", `{"route":"open-note","markdown":"# Open"}`, true, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish open-note = %d: %s", rec.Code, rec.Body.String())
	}
	rec = ts.do(t, http.MethodPut, "/api/pages/open-note", `{"password":"first-pass"}`, true, "")
	if rec.Code != http.StatusOK || decodeJSON(t, rec)["password_protected"] != true {
		t.Fatalf("protect update = %d: %s", rec.Code, rec.Body.String())
	}
	// And the public plane now gates it.
	if rec := ts.do(t, http.MethodGet, "/open-note", "", false, "3.3.3.3"); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Open") {
		t.Errorf("GET after protect = %d (content gated: %v)", rec.Code, !strings.Contains(rec.Body.String(), "Open"))
	}
}

// --- password gate, auth flow, rate limiting ---

func publishProtected(t *testing.T, ts *testServer, route, markdown, password string) {
	t.Helper()
	body := map[string]any{"route": route, "title": route, "markdown": markdown}
	if password != "" {
		body["password"] = password
	}
	b, _ := json.Marshal(body)
	rec := ts.do(t, http.MethodPost, "/api/pages", string(b), true, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish %s = %d: %s", route, rec.Code, rec.Body.String())
	}
}

func TestProtectedPageServesOnlyForm(t *testing.T) {
	ts := newTestServer(t)
	secret := "open-sesame"
	publishProtected(t, ts, "secret-note", "# Secret Content `classified`", secret)

	rec := ts.do(t, http.MethodGet, "/secret-note", "", false, "7.7.7.7")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET protected = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "password protected") {
		t.Error("password form not served")
	}
	if strings.Contains(body, "Secret Content") || strings.Contains(body, "classified") {
		t.Error("protected content leaked before auth")
	}
	if strings.Contains(body, "$argon2") || strings.Contains(body, secret) {
		t.Error("hash or plaintext password leaked")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
}

func TestAuthFlowSuccess(t *testing.T) {
	ts := newTestServer(t)
	publishProtected(t, ts, "gate", "# Behind the Gate", "right-password")

	// Wrong password.
	form := "password=wrong"
	rec := ts.doForm(t, http.MethodPost, "/gate/auth", form, "5.5.5.5")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Behind the Gate") {
		t.Error("content leaked on failed auth")
	}

	// Correct password: 303 + session cookie.
	rec = ts.doForm(t, http.MethodPost, "/gate/auth", "password=right-password", "5.5.5.6")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("correct password = %d, want 303; body %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(cookies))
	}
	ck := cookies[0]
	if !ck.HttpOnly || !ck.Secure || ck.Path != "/gate" {
		t.Errorf("cookie attrs wrong: %+v", ck)
	}

	// GET with the session cookie serves the content.
	req := httptest.NewRequest(http.MethodGet, "/gate", nil)
	req.Header.Set("Cookie", session.CookieName+"="+ck.Value)
	rec2 := httptest.NewRecorder()
	ts.e.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "Behind the Gate") {
		t.Errorf("GET with session = %d; body has content: %v", rec2.Code, strings.Contains(rec2.Body.String(), "Behind the Gate"))
	}

	// Session is route-scoped: the same cookie must NOT unlock a different
	// protected route.
	publishProtected(t, ts, "gate2", "# Other Secret", "other-pass")
	req = httptest.NewRequest(http.MethodGet, "/gate2", nil)
	req.Header.Set("Cookie", session.CookieName+"="+ck.Value)
	rec3 := httptest.NewRecorder()
	ts.e.ServeHTTP(rec3, req)
	if strings.Contains(rec3.Body.String(), "Other Secret") {
		t.Error("session cookie unlocked a different route")
	}
}

func TestAuthRateLimit(t *testing.T) {
	ts := newTestServer(t)
	publishProtected(t, ts, "limited", "# Limited", "real-pass")

	// 5 wrong attempts from one IP: all 401.
	for i := 1; i <= 5; i++ {
		rec := ts.doForm(t, http.MethodPost, "/limited/auth", "password=nope", "6.6.6.6")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i, rec.Code)
		}
	}
	// 6th within the window: 429, even with the correct password.
	rec := ts.doForm(t, http.MethodPost, "/limited/auth", "password=real-pass", "6.6.6.6")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("6th attempt = %d, want 429", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Limited") {
		t.Error("content leaked in rate-limited response")
	}
	// A different IP is not blocked (limiter is per IP+route).
	rec = ts.doForm(t, http.MethodPost, "/limited/auth", "password=real-pass", "6.6.6.7")
	if rec.Code != http.StatusSeeOther {
		t.Errorf("different IP = %d, want 303", rec.Code)
	}
}

func TestAuthOnUnprotectedPageRedirects(t *testing.T) {
	ts := newTestServer(t)
	publishProtected(t, ts, "open", "# Open", "")
	rec := ts.doForm(t, http.MethodPost, "/open/auth", "password=whatever", "4.4.4.4")
	if rec.Code != http.StatusSeeOther {
		t.Errorf("auth on unprotected page = %d, want 303", rec.Code)
	}
}

func TestAuthUnknownRoute404(t *testing.T) {
	ts := newTestServer(t)
	rec := ts.doForm(t, http.MethodPost, "/ghost/auth", "password=x", "4.4.4.4")
	if rec.Code != http.StatusNotFound {
		t.Errorf("auth on unknown route = %d, want 404", rec.Code)
	}
}

// --- persistence: served content survives a server restart ---

func TestPublishedPagesSurviveRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "restart.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	sessions, _ := session.NewManager([]byte("restart-secret"), time.Hour)
	e, _ := New(Deps{
		Store:    st,
		Token:    testToken,
		Sessions: sessions,
		Limiter:  ratelimit.New(5, time.Minute),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/pages", bytes.NewReader([]byte(`{"route":"durable","markdown":"# Still Here"}`)))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d: %s", rec.Code, rec.Body.String())
	}
	st.Close()

	// New server instance on the same DB file.
	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st2.Close()
	e2, _ := New(Deps{
		Store:    st2,
		Token:    testToken,
		Sessions: sessions,
		Limiter:  ratelimit.New(5, time.Minute),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	req2 := httptest.NewRequest(http.MethodGet, "/durable", nil)
	rec2 := httptest.NewRecorder()
	e2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "Still Here") {
		t.Errorf("GET after restart = %d (content present: %v)", rec2.Code, strings.Contains(rec2.Body.String(), "Still Here"))
	}
}

// --- malformed payloads ---

func TestMalformedPublishPayload(t *testing.T) {
	ts := newTestServer(t)
	rec := ts.do(t, http.MethodPost, "/api/pages", `{not json`, true, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed JSON = %d, want 400", rec.Code)
	}
	rec = ts.do(t, http.MethodPost, "/api/pages", `{"markdown":"x"}`, true, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing route = %d, want 400", rec.Code)
	}
}

// doForm posts a form body (password gate).
func (ts *testServer) doForm(t *testing.T, method, target, form, ip string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(form))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.RemoteAddr = ip + ":31337"
	rec := httptest.NewRecorder()
	ts.e.ServeHTTP(rec, req)
	return rec
}
