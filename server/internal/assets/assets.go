// Package assets stores immutable uploaded image assets on disk, named by
// content hash so identical uploads dedupe, and serves them back by filename.
//
// Only raster image formats are accepted (PNG, JPEG, GIF, WebP). SVG is
// deliberately rejected: it can carry scripts and would be an XSS vector on
// reader pages. Files are content-addressed and immutable once written, so
// they are safe to serve with far-future cache headers.
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MaxSize bounds a single uploaded asset (10 MB).
const MaxSize = 10 << 20

// MaxFilenameLen bounds stored asset filenames (16 hex chars + "-" + name).
const MaxFilenameLen = 160

// maxNameLen bounds the sanitized original-name portion of a filename.
const maxNameLen = 128

// Errors returned by Store operations.
var (
	// ErrInvalidType reports an unsupported or mislabeled upload. SVG and
	// every non-image format are rejected.
	ErrInvalidType = errors.New("assets: unsupported image type")
	// ErrNotFound reports an unknown or invalid filename on open.
	ErrNotFound = errors.New("assets: no such asset")
)

// imageTypes maps allowed file extensions to their Content-Type. Extension
// matching is case-insensitive.
var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// Store holds uploaded assets under a single directory.
type Store struct {
	dir string
}

// Open creates (if needed) the asset directory and returns a Store over it.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("assets: create dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the on-disk asset directory.
func (s *Store) Dir() string { return s.dir }

// Save validates and stores content, naming it after its content hash plus a
// sanitized version of originalName: first 16 hex chars of sha256 + "-" +
// sanitized name. If the file already exists it is returned as-is (existed =
// true) — assets are immutable, so the bytes are identical by construction.
// The upload's extension and magic bytes must agree on one of the allowed
// image types; anything else (including SVG) yields ErrInvalidType.
func (s *Store) Save(content []byte, originalName string) (filename string, existed bool, err error) {
	ext := strings.ToLower(filepath.Ext(originalName))
	wantType, ok := imageTypes[ext]
	if !ok {
		return "", false, fmt.Errorf("%w: %q is not a png, jpg, jpeg, gif, or webp file", ErrInvalidType, originalName)
	}
	// Defense in depth: the magic bytes must match the claimed extension, so
	// a mislabeled or polyglot file is rejected regardless of its name.
	if got := http.DetectContentType(content); got != wantType {
		return "", false, fmt.Errorf("%w: contents look like %s, not %s", ErrInvalidType, got, wantType)
	}

	sum := sha256.Sum256(content)
	filename = hex.EncodeToString(sum[:])[:16] + "-" + sanitizeName(originalName)
	path := filepath.Join(s.dir, filename)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return filename, true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("assets: create %s: %w", filename, err)
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return "", false, fmt.Errorf("assets: write %s: %w", filename, err)
	}
	if err := f.Close(); err != nil {
		return "", false, fmt.Errorf("assets: close %s: %w", filename, err)
	}
	return filename, false, nil
}

// Open opens the stored asset for reading. The filename must pass
// ValidFilename, which is the path-traversal guard: only filenames this
// package could have written are ever opened. Returns ErrNotFound for
// anything else.
func (s *Store) Open(filename string) (*os.File, error) {
	if !ValidFilename(filename) {
		return nil, ErrNotFound
	}
	f, err := os.Open(filepath.Join(s.dir, filename))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("assets: open %s: %w", filename, err)
	}
	return f, nil
}

// ContentType returns the Content-Type for a stored asset filename (by
// extension). Empty string for unknown extensions; ValidFilename filenames
// always have a known one.
func ContentType(filename string) string {
	return imageTypes[strings.ToLower(filepath.Ext(filename))]
}

// ValidFilename reports whether name is a filename this package could have
// written: ASCII [A-Za-z0-9._-] only (no separators, no traversal), with an
// allowed image extension.
func ValidFilename(name string) bool {
	if name == "" || len(name) > MaxFilenameLen || name == "." || name == ".." {
		return false
	}
	if _, ok := imageTypes[strings.ToLower(filepath.Ext(name))]; !ok {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// sanitizeName reduces an uploaded filename to the plain [A-Za-z0-9._-]
// charset, drops any path components, and bounds its length while keeping the
// extension. The fallback for fully unsalvageable names is "image" + ext.
func sanitizeName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.TrimLeft(b.String(), ".-")
	if out == "" {
		out = "image" + ext
	}
	if len(out) > maxNameLen {
		if e := filepath.Ext(out); len(e) < len(out) {
			out = out[:maxNameLen-len(e)] + e
		} else {
			out = out[:maxNameLen]
		}
	}
	return out
}
