package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
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

	updated, err := s.Update("note", "New Title", "new md", "new html", "", "note-123")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "New Title" || updated.ContentMD != "new md" || updated.PasswordHash != "" || updated.SourceNoteID != "note-123" {
		t.Errorf("updated page = %+v", updated)
	}
	if updated.UpdatedAt.Before(created.CreatedAt) {
		t.Errorf("UpdatedAt %v before created_at %v", updated.UpdatedAt, created.CreatedAt)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Error("created_at changed on update")
	}

	if _, err := s.Update("missing", "t", "m", "h", "", ""); !errors.Is(err, ErrNotFound) {
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
