package assets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngBytes is a minimal valid PNG (1x1 transparent pixel).
var pngBytes = []byte{
	0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', // PNG magic
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
	0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0d, 0x0a, 0x2d, 0xb4,
	0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// gifBytes is a minimal valid GIF (1x1 transparent pixel).
var gifBytes = []byte{
	'G', 'I', 'F', '8', '9', 'a', 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
	0xff, 0xff, 0xff, 0x00, 0x00, 0x00, '!', 0xf9, 0x04, 0x01, 0x00, 0x00,
	0x00, 0x00, ',', 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00,
	0x02, 0x02, 'D', 0x01, 0x00, ';',
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestSaveAndDedupe(t *testing.T) {
	s := openTemp(t)
	name, existed, err := s.Save(pngBytes, "photo.png")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if existed {
		t.Error("first Save reported existed = true")
	}
	if !strings.HasSuffix(name, "-photo.png") || len(strings.SplitN(name, "-", 2)[0]) != 16 {
		t.Errorf("filename = %q, want <16 hex chars>-photo.png", name)
	}

	// Same content + same name: dedupes to the existing file.
	name2, existed, err := s.Save(pngBytes, "photo.png")
	if err != nil {
		t.Fatalf("Save again: %v", err)
	}
	if !existed || name2 != name {
		t.Errorf("re-Save = (%q, %v), want (%q, true)", name2, existed, name)
	}

	// Same content under a different original name is a distinct file.
	name3, existed, err := s.Save(pngBytes, "renamed.png")
	if err != nil {
		t.Fatalf("Save renamed: %v", err)
	}
	if existed || name3 == name {
		t.Errorf("renamed Save = (%q, %v)", name3, existed)
	}
}

func TestSaveRejectsNonImages(t *testing.T) {
	s := openTemp(t)

	svg := []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"><script>alert(1)</script></svg>")
	if _, _, err := s.Save(svg, "evil.svg"); !errors.Is(err, ErrInvalidType) {
		t.Errorf("svg err = %v, want ErrInvalidType", err)
	}
	if _, _, err := s.Save([]byte("plain text"), "notes.txt"); !errors.Is(err, ErrInvalidType) {
		t.Errorf("txt err = %v, want ErrInvalidType", err)
	}
	if _, _, err := s.Save(pngBytes, "no-extension"); !errors.Is(err, ErrInvalidType) {
		t.Errorf("extensionless err = %v, want ErrInvalidType", err)
	}
	// Mislabeled: PNG bytes claiming to be a jpg.
	if _, _, err := s.Save(pngBytes, "mislabeled.jpg"); !errors.Is(err, ErrInvalidType) {
		t.Errorf("mislabeled err = %v, want ErrInvalidType", err)
	}

	// Nothing was written for rejected uploads.
	entries, _ := os.ReadDir(s.Dir())
	if len(entries) != 0 {
		t.Errorf("rejected uploads left %d files behind", len(entries))
	}
}

func TestSaveSanitizesNames(t *testing.T) {
	s := openTemp(t)
	name, _, err := s.Save(gifBytes, "../../my trip: 2026!.gif")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !ValidFilename(name) {
		t.Errorf("filename %q is not servable", name)
	}
	if !strings.Contains(name, "-my-trip--2026-.gif") {
		t.Errorf("filename = %q, want sanitized original name kept", name)
	}
}

func TestOpenAndValidFilename(t *testing.T) {
	s := openTemp(t)
	name, _, err := s.Save(pngBytes, "pic.png")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	f, err := s.Open(name)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	f.Close()

	for _, bad := range []string{
		name + "/../" + name,              // traversal via separators
		"..",                              // parent dir
		"../" + name,                      // parent dir with valid tail
		"subdir/" + name,                  // subdirectory
		"upload.svg",                      // disallowed extension
		strings.Repeat("a", 200) + ".png", // too long
		"",                                // empty
	} {
		if ValidFilename(bad) {
			t.Errorf("ValidFilename(%q) = true, want false", bad)
		}
		if _, err := s.Open(bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("Open(%q) err = %v, want ErrNotFound", bad, err)
		}
	}

	// A well-formed name that was never uploaded is valid shape but absent.
	if !ValidFilename("no-such-file.png") {
		t.Error("ValidFilename(no-such-file.png) = false, want true")
	}
	if _, err := s.Open("no-such-file.png"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Open(no-such-file.png) err = %v, want ErrNotFound", err)
	}
}

func TestContentType(t *testing.T) {
	cases := map[string]string{
		"a.png":  "image/png",
		"a.PNG":  "image/png",
		"a.jpg":  "image/jpeg",
		"a.jpeg": "image/jpeg",
		"a.gif":  "image/gif",
		"a.webp": "image/webp",
		"a.txt":  "",
	}
	for name, want := range cases {
		if got := ContentType(name); got != want {
			t.Errorf("ContentType(%q) = %q, want %q", name, got, want)
		}
	}
}
