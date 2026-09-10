package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestThemeAuthRequired(t *testing.T) {
	ts := newTestServer(t)
	if rec := ts.do(t, http.MethodGet, "/api/theme", "", false, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/theme without token = %d, want 401", rec.Code)
	}
	if rec := ts.do(t, http.MethodPost, "/api/theme", `{"css":"body{}"}`, false, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /api/theme without token = %d, want 401", rec.Code)
	}
}

func TestThemeGetSetRoundtrip(t *testing.T) {
	ts := newTestServer(t)

	// Empty state.
	rec := ts.do(t, http.MethodGet, "/api/theme", "", true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/theme = %d: %s", rec.Code, rec.Body.String())
	}
	theme := decodeJSON(t, rec)
	if theme["css"] != "" || theme["name"] != "default" {
		t.Errorf("empty theme = %v, want {css:%q name:%q}", theme, "", "default")
	}
	if updated, ok := theme["updated_at"].(string); !ok || updated == "" {
		t.Errorf("empty theme updated_at = %v, want non-empty timestamp", theme["updated_at"])
	}

	// Set a theme.
	rec = ts.do(t, http.MethodPost, "/api/theme", `{"css":"body { color: purple; }","name":"midnight"}`, true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/theme = %d: %s", rec.Code, rec.Body.String())
	}
	set := decodeJSON(t, rec)
	if set["css"] != "body { color: purple; }" || set["name"] != "midnight" {
		t.Errorf("set response = %v", set)
	}

	rec = ts.do(t, http.MethodGet, "/api/theme", "", true, "")
	got := decodeJSON(t, rec)
	if got["css"] != "body { color: purple; }" || got["name"] != "midnight" {
		t.Errorf("GET after set = %v", got)
	}
	if updated, _ := got["updated_at"].(string); updated != "" {
		if _, err := time.Parse(time.RFC3339, updated); err != nil {
			t.Errorf("updated_at %q is not RFC3339: %v", updated, err)
		}
	} else {
		t.Error("updated_at missing after set")
	}

	// Reset to the default styling (empty css, no name).
	rec = ts.do(t, http.MethodPost, "/api/theme", `{"css":""}`, true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reset = %d: %s", rec.Code, rec.Body.String())
	}
	reset := decodeJSON(t, rec)
	if reset["css"] != "" || reset["name"] != "default" {
		t.Errorf("reset response = %v", reset)
	}
}

func TestThemeSizeCap(t *testing.T) {
	ts := newTestServer(t)
	big := `{"css":"` + strings.Repeat("a", 256*1024+1) + `"}`
	if rec := ts.do(t, http.MethodPost, "/api/theme", big, true, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("oversize theme = %d, want 400", rec.Code)
	}
	at := `{"css":"` + strings.Repeat("a", 256*1024) + `"}`
	if rec := ts.do(t, http.MethodPost, "/api/theme", at, true, ""); rec.Code != http.StatusOK {
		t.Errorf("exactly-256KB theme = %d, want 200", rec.Code)
	}
	// Per-page override carries the same cap.
	if rec := ts.do(t, http.MethodPost, "/api/pages", big, true, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("oversize theme_css on publish = %d, want 400", rec.Code)
	}
}

// TestThemePrecedenceInServedHTML pins the public-render precedence:
// per-page theme_css > global theme css > built-in default.
func TestThemePrecedenceInServedHTML(t *testing.T) {
	ts := newTestServer(t)
	const (
		globalCSS   = "body { color: purple; }"
		overrideCSS = "body { background: #111111; }"
	)

	publish := func(route, themeCSS string) {
		t.Helper()
		body := `{"route":"` + route + `","markdown":"# ` + route + `"`
		if themeCSS != "" {
			body += `,"theme_css":"` + themeCSS + `"`
		}
		body += `}`
		rec := ts.do(t, http.MethodPost, "/api/pages", body, true, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("publish %s = %d: %s", route, rec.Code, rec.Body.String())
		}
	}

	publish("themed", overrideCSS)
	publish("plain", "")

	getBody := func(route string) string {
		t.Helper()
		rec := ts.do(t, http.MethodGet, "/"+route, "", false, "8.8.8.8")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /%s = %d: %s", route, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	// No global theme yet: default CSS everywhere; the override already wins.
	if body := getBody("plain"); !strings.Contains(body, "color-scheme") {
		t.Error("plain page missing default stylesheet before any theme is set")
	}
	if body := getBody("themed"); !strings.Contains(body, overrideCSS) || strings.Contains(body, "color-scheme") {
		t.Error("per-page override did not replace the default stylesheet")
	}

	// Set a global theme: non-override pages re-render under it, the override
	// page is untouched.
	if rec := ts.do(t, http.MethodPost, "/api/theme", `{"css":"`+globalCSS+`","name":"g"}`, true, ""); rec.Code != http.StatusOK {
		t.Fatalf("set theme = %d: %s", rec.Code, rec.Body.String())
	}
	if body := getBody("plain"); !strings.Contains(body, globalCSS) || strings.Contains(body, "color-scheme") {
		t.Error("global theme did not replace the default on non-override pages")
	}
	if body := getBody("themed"); !strings.Contains(body, overrideCSS) || strings.Contains(body, globalCSS) {
		t.Error("global theme overrode a per-page theme_css")
	}

	// Pages published after the theme is set pick it up.
	publish("later", "")
	if body := getBody("later"); !strings.Contains(body, globalCSS) {
		t.Error("page published after theme set did not use the global theme")
	}

	// Resetting the theme returns non-override pages to the default.
	if rec := ts.do(t, http.MethodPost, "/api/theme", `{"css":""}`, true, ""); rec.Code != http.StatusOK {
		t.Fatalf("reset theme = %d", rec.Code)
	}
	if body := getBody("plain"); !strings.Contains(body, "color-scheme") || strings.Contains(body, globalCSS) {
		t.Error("theme reset did not restore the default stylesheet")
	}
	if body := getBody("themed"); !strings.Contains(body, overrideCSS) {
		t.Error("theme reset clobbered a per-page override")
	}
}

// TestPasswordFormKeepsDefaultStylesheet: however aggressive the theme, the
// password-entry form always carries its own fixed stylesheet.
func TestPasswordFormKeepsDefaultStylesheet(t *testing.T) {
	ts := newTestServer(t)
	if rec := ts.do(t, http.MethodPost, "/api/theme", `{"css":"body { display: none; } form { display: none; }"}`, true, ""); rec.Code != http.StatusOK {
		t.Fatalf("set theme = %d", rec.Code)
	}
	publishProtected(t, ts, "vault", "# Behind the Lock", "pass")
	rec := ts.do(t, http.MethodGet, "/vault", "", false, "8.8.8.8")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET protected = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "password protected") {
		t.Fatal("password form not served")
	}
	if !strings.Contains(body, "display: flex") {
		t.Error("password form lost its default stylesheet")
	}
	if strings.Contains(body, "display: none") {
		t.Error("custom theme CSS leaked into the password form")
	}
}

func TestPageThemeOverrideFields(t *testing.T) {
	ts := newTestServer(t)
	const overrideCSS = "body { background: #abcdef; } /* UNIQUE-OVERRIDE-1 */"

	// Publish with an override.
	rec := ts.do(t, http.MethodPost, "/api/pages",
		`{"route":"over","markdown":"# Over","theme_css":"`+overrideCSS+`"}`, true, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d: %s", rec.Code, rec.Body.String())
	}
	if created := decodeJSON(t, rec); created["theme_override"] != true {
		t.Errorf("publish theme_override = %v, want true", created["theme_override"])
	}
	if strings.Contains(rec.Body.String(), overrideCSS) {
		t.Error("publish response leaked theme_css itself")
	}

	// Publish without.
	rec = ts.do(t, http.MethodPost, "/api/pages", `{"route":"under","markdown":"# Under"}`, true, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish under = %d", rec.Code)
	}
	if plain := decodeJSON(t, rec); plain["theme_override"] != false {
		t.Errorf("publish theme_override = %v, want false", plain["theme_override"])
	}

	// List: theme_override flags, never the CSS, never password hashes.
	rec = ts.do(t, http.MethodGet, "/api/pages", "", true, "")
	body := rec.Body.String()
	if !strings.Contains(body, `"theme_override":true`) || !strings.Contains(body, `"theme_override":false`) {
		t.Errorf("list missing theme_override flags: %s", body)
	}
	if strings.Contains(body, "theme_css") || strings.Contains(body, overrideCSS) || strings.Contains(body, "password_hash") {
		t.Error("list leaked theme_css, its contents, or password_hash")
	}

	// PUT with theme_css absent keeps the override.
	rec = ts.do(t, http.MethodPut, "/api/pages/over", `{"markdown":"# Over v2"}`, true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("put keep = %d: %s", rec.Code, rec.Body.String())
	}
	if kept := decodeJSON(t, rec); kept["theme_override"] != true {
		t.Errorf("theme_override after field-absent put = %v, want true", kept["theme_override"])
	}
	if rec := ts.do(t, http.MethodGet, "/over", "", false, "8.8.8.8"); !strings.Contains(rec.Body.String(), overrideCSS) {
		t.Error("override css lost from served page after unrelated update")
	}

	// PUT with explicit null clears the override (back to default styling).
	rec = ts.do(t, http.MethodPut, "/api/pages/over", `{"theme_css":null}`, true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("put clear = %d: %s", rec.Code, rec.Body.String())
	}
	if cleared := decodeJSON(t, rec); cleared["theme_override"] != false {
		t.Errorf("theme_override after null = %v, want false", cleared["theme_override"])
	}
	if rec := ts.do(t, http.MethodGet, "/over", "", false, "8.8.8.8"); !strings.Contains(rec.Body.String(), "color-scheme") {
		t.Error("cleared override did not fall back to the default stylesheet")
	}
}
