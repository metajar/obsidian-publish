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
// v2 adds the per-page theme_css column and the theme singleton table.
const schemaVersion = 2

// MaxRouteLen bounds route slug length.
const MaxRouteLen = 64

// reservedRoutes can never be published: they collide with server endpoints
// or well-known crawl paths.
var reservedRoutes = map[string]bool{
	"api":         true,
	"assets":      true,
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
	ThemeCSS     string // optional per-page theme override; "" = use global theme
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PageMeta is the metadata projection returned by List — deliberately excludes
// content, password hash, and theme CSS.
type PageMeta struct {
	Route             string    `json:"route"`
	Title             string    `json:"title"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	PasswordProtected bool      `json:"password_protected"`
	SourceNoteID      string    `json:"source_note_id,omitempty"`
	ThemeOverride     bool      `json:"theme_override"`
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

	if version < 1 {
		// Fresh database: create the pages table at the current shape.
		const createPages = `CREATE TABLE IF NOT EXISTS pages (
			route          TEXT PRIMARY KEY,
			title          TEXT NOT NULL,
			content_md     TEXT NOT NULL,
			content_html   TEXT NOT NULL,
			password_hash  TEXT,
			source_note_id TEXT,
			theme_css      TEXT,
			created_at     TEXT NOT NULL,
			updated_at     TEXT NOT NULL
		)`
		if _, err := s.db.Exec(createPages); err != nil {
			return fmt.Errorf("store: create pages table: %w", err)
		}
	}
	if version < 2 {
		// v1 -> v2: per-page theme override column + global theme singleton.
		if version == 1 {
			if _, err := s.db.Exec(`ALTER TABLE pages ADD COLUMN theme_css TEXT`); err != nil {
				return fmt.Errorf("store: add theme_css column: %w", err)
			}
		}
		const createTheme = `CREATE TABLE IF NOT EXISTS theme (
			id         INTEGER PRIMARY KEY CHECK (id = 1),
			css        TEXT NOT NULL,
			name       TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`
		if _, err := s.db.Exec(createTheme); err != nil {
			return fmt.Errorf("store: create theme table: %w", err)
		}
		// Seed the singleton with the empty default so GET /api/theme always
		// has a stable row to report.
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO theme (id, css, name, updated_at)
			VALUES (1, '', 'default', ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("store: seed theme row: %w", err)
		}
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
		(route, title, content_md, content_html, password_hash, source_note_id, theme_css, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Route, p.Title, p.ContentMD, p.ContentHTML, nullString(p.PasswordHash), nullString(p.SourceNoteID),
		nullString(p.ThemeCSS),
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
		COALESCE(password_hash, ''), COALESCE(source_note_id, ''), COALESCE(theme_css, ''),
		created_at, updated_at
		FROM pages WHERE route = ?`, route)
	return scanPage(row)
}

// List returns metadata for all pages ordered by route. It never returns
// content or password hashes.
func (s *Store) List() ([]PageMeta, error) {
	rows, err := s.db.Query(`SELECT route, title, COALESCE(source_note_id, ''),
		created_at, updated_at, password_hash IS NOT NULL, COALESCE(theme_css, '') != ''
		FROM pages ORDER BY route`)
	if err != nil {
		return nil, fmt.Errorf("store: list pages: %w", err)
	}
	defer rows.Close()
	var out []PageMeta
	for rows.Next() {
		var m PageMeta
		var created, updated string
		if err := rows.Scan(&m.Route, &m.Title, &m.SourceNoteID, &created, &updated, &m.PasswordProtected, &m.ThemeOverride); err != nil {
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
// password hash, source note id, theme override), stamping updated_at. The
// route and created_at never change. Returns ErrNotFound for an unknown route.
func (s *Store) Update(route, title, contentMD, contentHTML, passwordHash, sourceNoteID, themeCSS string) (*Page, error) {
	res, err := s.db.Exec(`UPDATE pages
		SET title = ?, content_md = ?, content_html = ?, password_hash = ?, source_note_id = ?, theme_css = ?, updated_at = ?
		WHERE route = ?`,
		title, contentMD, contentHTML, nullString(passwordHash), nullString(sourceNoteID), nullString(themeCSS),
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

// Theme is the global site theme. At most one row exists (id = 1).
type Theme struct {
	CSS       string
	Name      string
	UpdatedAt time.Time
}

// DefaultThemeName labels the empty theme, i.e. the built-in default styling.
const DefaultThemeName = "default"

// GetTheme returns the global site theme. The row is seeded at migration
// time, so the empty state is CSS "" with DefaultThemeName.
func (s *Store) GetTheme() (Theme, error) {
	var t Theme
	var updated string
	err := s.db.QueryRow(`SELECT css, name, updated_at FROM theme WHERE id = 1`).
		Scan(&t.CSS, &t.Name, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		// Defensive: the migration seeds the singleton row.
		return Theme{Name: DefaultThemeName}, nil
	}
	if err != nil {
		return Theme{}, fmt.Errorf("store: get theme: %w", err)
	}
	if t.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Theme{}, fmt.Errorf("store: parse theme updated_at: %w", err)
	}
	return t, nil
}

// SetTheme upserts the global site theme. An empty name means DefaultThemeName.
func (s *Store) SetTheme(css, name string) (Theme, error) {
	if name == "" {
		name = DefaultThemeName
	}
	_, err := s.db.Exec(`INSERT INTO theme (id, css, name, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET css = excluded.css, name = excluded.name, updated_at = excluded.updated_at`,
		css, name, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Theme{}, fmt.Errorf("store: set theme: %w", err)
	}
	return s.GetTheme()
}

// SetRendered replaces only the stored rendered HTML of a page. It is used
// when a theme change re-renders pages; updated_at is deliberately untouched
// because a theme change is not a content change. Returns ErrNotFound for an
// unknown route.
func (s *Store) SetRendered(route, contentHTML string) error {
	res, err := s.db.Exec(`UPDATE pages SET content_html = ? WHERE route = ?`, contentHTML, route)
	if err != nil {
		return fmt.Errorf("store: set rendered html: %w", err)
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
		&p.PasswordHash, &p.SourceNoteID, &p.ThemeCSS, &created, &updated)
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
