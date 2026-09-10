// Package store persists published pages to a single SQLite file using the
// pure-Go modernc.org/sqlite driver (no CGo). Schema versioning uses the
// SQLite user_version pragma.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// Errors returned by Store operations.
var (
	ErrNotFound     = errors.New("store: page not found")
	ErrConflict     = errors.New("store: route already taken")
	ErrInvalidRoute = errors.New("store: invalid route")
)

// schemaVersion is the current pages schema version (PRAGMA user_version).
const schemaVersion = 1

// MaxRouteLen bounds route slug length.
const MaxRouteLen = 64

// reservedRoutes can never be published: they collide with server endpoints
// or well-known crawl paths.
var reservedRoutes = map[string]bool{
	"api":         true,
	"favicon.ico": true,
	"robots.txt":  true,
}

// Page is a full stored page record.
type Page struct {
	Route        string
	Title        string
	ContentMD    string
	ContentHTML  string
	PasswordHash string // "" when the page is not protected
	SourceNoteID string // optional, plugin bookkeeping
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PageMeta is the metadata projection returned by List — deliberately excludes
// content and password hash.
type PageMeta struct {
	Route             string    `json:"route"`
	Title             string    `json:"title"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	PasswordProtected bool      `json:"password_protected"`
	SourceNoteID      string    `json:"source_note_id,omitempty"`
}

// Store wraps the SQLite database holding published pages.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and runs migrations.
func Open(path string) (*Store, error) {
	// Single connection avoids SQLITE_BUSY under concurrent access; this is a
	// single-owner personal server. WAL keeps reads cheap.
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("store: read user_version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("store: database schema version %d is newer than supported version %d; upgrade the server", version, schemaVersion)
	}
	if version == schemaVersion {
		return nil
	}
	const createPages = `CREATE TABLE IF NOT EXISTS pages (
		route          TEXT PRIMARY KEY,
		title          TEXT NOT NULL,
		content_md     TEXT NOT NULL,
		content_html   TEXT NOT NULL,
		password_hash  TEXT,
		source_note_id TEXT,
		created_at     TEXT NOT NULL,
		updated_at     TEXT NOT NULL
	)`
	if _, err := s.db.Exec(createPages); err != nil {
		return fmt.Errorf("store: create pages table: %w", err)
	}
	if _, err := s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("store: set user_version: %w", err)
	}
	return nil
}

// ValidRoute reports whether route is a publishable URL-safe slug:
// 1..64 chars of [a-z0-9-], starting and ending with an alphanumeric, and
// not a reserved route. This is the server-side security check — the
// plugin's client-side validation is UX only.
func ValidRoute(route string) bool {
	if len(route) < 1 || len(route) > MaxRouteLen || reservedRoutes[route] {
		return false
	}
	for i := 0; i < len(route); i++ {
		c := route[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-':
			if i == 0 || i == len(route)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Create inserts a new page. Returns ErrConflict if the route exists and
// ErrInvalidRoute for an unpublishable route.
func (s *Store) Create(p Page) error {
	if !ValidRoute(p.Route) {
		return ErrInvalidRoute
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = p.CreatedAt
	_, err := s.db.Exec(`INSERT INTO pages
		(route, title, content_md, content_html, password_hash, source_note_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Route, p.Title, p.ContentMD, p.ContentHTML, nullString(p.PasswordHash), nullString(p.SourceNoteID),
		p.CreatedAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrConflict
		}
		return fmt.Errorf("store: insert page: %w", err)
	}
	return nil
}

// Get returns the page for route, or ErrNotFound.
func (s *Store) Get(route string) (*Page, error) {
	row := s.db.QueryRow(`SELECT route, title, content_md, content_html,
		COALESCE(password_hash, ''), COALESCE(source_note_id, ''), created_at, updated_at
		FROM pages WHERE route = ?`, route)
	return scanPage(row)
}

// List returns metadata for all pages ordered by route. It never returns
// content or password hashes.
func (s *Store) List() ([]PageMeta, error) {
	rows, err := s.db.Query(`SELECT route, title, COALESCE(source_note_id, ''),
		created_at, updated_at, password_hash IS NOT NULL FROM pages ORDER BY route`)
	if err != nil {
		return nil, fmt.Errorf("store: list pages: %w", err)
	}
	defer rows.Close()
	var out []PageMeta
	for rows.Next() {
		var m PageMeta
		var created, updated string
		if err := rows.Scan(&m.Route, &m.Title, &m.SourceNoteID, &created, &updated, &m.PasswordProtected); err != nil {
			return nil, fmt.Errorf("store: scan page meta: %w", err)
		}
		if m.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse created_at: %w", err)
		}
		if m.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
			return nil, fmt.Errorf("store: parse updated_at: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Update replaces the mutable fields of an existing page (title, content,
// password hash, source note id), stamping updated_at. The route and
// created_at never change. Returns ErrNotFound for an unknown route.
func (s *Store) Update(route, title, contentMD, contentHTML, passwordHash, sourceNoteID string) (*Page, error) {
	res, err := s.db.Exec(`UPDATE pages
		SET title = ?, content_md = ?, content_html = ?, password_hash = ?, source_note_id = ?, updated_at = ?
		WHERE route = ?`,
		title, contentMD, contentHTML, nullString(passwordHash), nullString(sourceNoteID),
		time.Now().UTC().Format(time.RFC3339Nano), route)
	if err != nil {
		return nil, fmt.Errorf("store: update page: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return nil, ErrNotFound
	}
	return s.Get(route)
}

// Delete removes the page for route. Returns ErrNotFound for an unknown route.
func (s *Store) Delete(route string) error {
	res, err := s.db.Exec(`DELETE FROM pages WHERE route = ?`, route)
	if err != nil {
		return fmt.Errorf("store: delete page: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanPage(row *sql.Row) (*Page, error) {
	var p Page
	var created, updated string
	err := row.Scan(&p.Route, &p.Title, &p.ContentMD, &p.ContentHTML,
		&p.PasswordHash, &p.SourceNoteID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan page: %w", err)
	}
	if p.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return nil, fmt.Errorf("store: parse created_at: %w", err)
	}
	if p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return nil, fmt.Errorf("store: parse updated_at: %w", err)
	}
	return &p, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
