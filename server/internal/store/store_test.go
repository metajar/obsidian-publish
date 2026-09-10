package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for the migration test
)

func testPage(route string) Page {
	return Page{
		Route:        route,
		Title:        "Test Page",
		ContentMD:    "# Hello\n\nworld",
		ContentHTML:  "<h1>Hello</h1><p>world</p>",
		PasswordHash: "hash-value",
	}
}

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestValidRoute(t *testing.T) {
	valid := []string{"a", "note", "my-note-2", "2024-recap", "a1b2c3", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789"[:64]}
	for _, r := range valid {
		if !ValidRoute(r) {
			t.Errorf("ValidRoute(%q) = false, want true", r)
		}
	}
	invalid := []string{
		"",            // empty
		"-leading",    // leading hyphen
		"trailing-",   // trailing hyphen
		"Upper",       // uppercase
		"under_score", // underscore
		"spa ce",      // space
		"slash/ed",    // path separator
		"dot.ted",     // dot
		"api",         // reserved
		"favicon.ico", // reserved
		"robots.txt",  // reserved
		"percent%20",  // percent
		"unicode-ü",   // non-ascii
		"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789"[:65], // too long
		";inject", // metachar
	}
	for _, r := range invalid {
		if ValidRoute(r) {
			t.Errorf("ValidRoute(%q) = true, want false", r)
		}
	}
}

func TestCreateGetConflict(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Create(testPage("my-note")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(testPage("my-note")); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate Create err = %v, want ErrConflict", err)
	}
	if err := s.Create(testPage("Bad Route")); !errors.Is(err, ErrInvalidRoute) {
		t.Errorf("invalid-route Create err = %v, want ErrInvalidRoute", err)
	}
	got, err := s.Get("my-note")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "Test Page" || got.PasswordHash != "hash-value" || got.ContentMD != "# Hello\n\nworld" {
		t.Errorf("Get returned %+v", got)
	}
	if _, err := s.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing err = %v, want ErrNotFound", err)
	}
}

func TestListOmitsSecrets(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Create(testPage("a")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(testPage("b")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	metas, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(metas))
	}
	for _, m := range metas {
		if m.PasswordProtected != true {
			t.Errorf("PageMeta.PasswordProtected = false, want true")
		}
	}
}

func TestUpdateAndDelete(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Create(testPage("note")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	created, _ := s.Get("note")

	updated, err := s.Update("note", "New Title", "new md", "new html", "", "note-123", "body{color:red}")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "New Title" || updated.ContentMD != "new md" || updated.PasswordHash != "" || updated.SourceNoteID != "note-123" || updated.ThemeCSS != "body{color:red}" {
		t.Errorf("updated page = %+v", updated)
	}
	if updated.UpdatedAt.Before(created.CreatedAt) {
		t.Errorf("UpdatedAt %v before created_at %v", updated.UpdatedAt, created.CreatedAt)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Error("created_at changed on update")
	}

	if _, err := s.Update("missing", "t", "m", "h", "", "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update missing err = %v, want ErrNotFound", err)
	}
	if err := s.Delete("note"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("note"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete err = %v, want ErrNotFound", err)
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s1.Create(testPage("survivor")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.Get("survivor")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Title != "Test Page" {
		t.Errorf("page lost across reopen: %+v", got)
	}
}

func TestTimestampsStoredUTC(t *testing.T) {
	s, _ := openTemp(t)
	before := time.Now().UTC().Add(-time.Second)
	if err := s.Create(testPage("ts")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := s.Get("ts")
	if got.CreatedAt.Before(before) || got.CreatedAt.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("CreatedAt = %v, expected ~now UTC", got.CreatedAt)
	}
}

// --- theme singleton ---

func TestThemeRoundtrip(t *testing.T) {
	s, _ := openTemp(t)

	// Empty state: css "", name "default", seeded updated_at.
	theme, err := s.GetTheme()
	if err != nil {
		t.Fatalf("GetTheme: %v", err)
	}
	if theme.CSS != "" || theme.Name != DefaultThemeName || theme.UpdatedAt.IsZero() {
		t.Errorf("empty theme = %+v, want {CSS:%q Name:%q UpdatedAt:<non-zero>}", theme, theme.CSS, DefaultThemeName)
	}

	theme, err = s.SetTheme("body { color: purple; }", "")
	if err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	if theme.CSS != "body { color: purple; }" || theme.Name != DefaultThemeName {
		t.Errorf("SetTheme default name = %+v", theme)
	}
	got, err := s.GetTheme()
	if err != nil {
		t.Fatalf("GetTheme: %v", err)
	}
	if got.CSS != theme.CSS || got.Name != theme.Name || !got.UpdatedAt.Equal(theme.UpdatedAt) {
		t.Errorf("GetTheme = %+v, want %+v", got, theme)
	}

	// Reset to default styling with an explicit name.
	got, err = s.SetTheme("", "midnight")
	if err != nil {
		t.Fatalf("SetTheme reset: %v", err)
	}
	if got.CSS != "" || got.Name != "midnight" {
		t.Errorf("SetTheme reset = %+v", got)
	}
}

func TestSetRendered(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Create(testPage("r")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	created, _ := s.Get("r")
	if err := s.SetRendered("r", "<h1>re-rendered</h1>"); err != nil {
		t.Fatalf("SetRendered: %v", err)
	}
	got, _ := s.Get("r")
	if got.ContentHTML != "<h1>re-rendered</h1>" {
		t.Errorf("ContentHTML = %q", got.ContentHTML)
	}
	if !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Error("SetRendered changed updated_at; a theme change is not a content change")
	}
	if err := s.SetRendered("missing", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetRendered missing err = %v, want ErrNotFound", err)
	}
}

// TestMigrationV1ToV2 builds a database at schema v1 (pages without
// theme_css, no theme table) and checks that Open migrates it in place.
func TestMigrationV1ToV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE pages (
		route          TEXT PRIMARY KEY,
		title          TEXT NOT NULL,
		content_md     TEXT NOT NULL,
		content_html   TEXT NOT NULL,
		password_hash  TEXT,
		source_note_id TEXT,
		created_at     TEXT NOT NULL,
		updated_at     TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create v1 pages: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO pages
		(route, title, content_md, content_html, password_hash, source_note_id, created_at, updated_at)
		VALUES ('old-page', 'Old', '# Old', '<h1>Old</h1>', NULL, NULL, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert v1 page: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open (migrate): %v", err)
	}
	t.Cleanup(func() { s.Close() })

	old, err := s.Get("old-page")
	if err != nil {
		t.Fatalf("Get migrated page: %v", err)
	}
	if old.Title != "Old" || old.ThemeCSS != "" {
		t.Errorf("migrated page = %+v", old)
	}

	// New-shape operations work on the migrated database.
	p := testPage("new-page")
	p.ThemeCSS = "body { color: purple; }"
	if err := s.Create(p); err != nil {
		t.Fatalf("Create with theme_css: %v", err)
	}
	got, _ := s.Get("new-page")
	if got.ThemeCSS != "body { color: purple; }" {
		t.Errorf("ThemeCSS = %q", got.ThemeCSS)
	}

	metas, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byRoute := map[string]PageMeta{}
	for _, m := range metas {
		byRoute[m.Route] = m
	}
	if byRoute["old-page"].ThemeOverride || !byRoute["new-page"].ThemeOverride {
		t.Errorf("List ThemeOverride: old-page = %v (want false), new-page = %v (want true)",
			byRoute["old-page"].ThemeOverride, byRoute["new-page"].ThemeOverride)
	}

	// The theme singleton is seeded.
	theme, err := s.GetTheme()
	if err != nil {
		t.Fatalf("GetTheme: %v", err)
	}
	if theme.CSS != "" || theme.Name != DefaultThemeName {
		t.Errorf("migrated theme = %+v", theme)
	}
}
