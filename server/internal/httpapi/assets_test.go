package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"obsidian-publish/server/internal/assets"
)

// pngBytes is a minimal valid PNG (1x1 transparent pixel).
var pngBytes = []byte{
	0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
	0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0d, 0x0a, 0x2d, 0xb4,
	0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// doMultipart uploads a multipart body. api decides the bearer token.
func (ts *testServer) doMultipart(t *testing.T, field, filename string, content []byte, api bool) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if filename == "" {
		_ = w.WriteField(field, "not-a-file")
	} else {
		fw, err := w.CreateFormFile(field, filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("write form file: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/assets", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	if api {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+testToken)
	}
	rec := httptest.NewRecorder()
	ts.e.ServeHTTP(rec, req)
	return rec
}

func TestAssetUploadAuthRequired(t *testing.T) {
	ts := newTestServer(t)
	if rec := ts.doMultipart(t, "file", "pic.png", pngBytes, false); rec.Code != http.StatusUnauthorized {
		t.Errorf("upload without token = %d, want 401", rec.Code)
	}
}

func TestAssetUploadHappyPathAndDedupe(t *testing.T) {
	ts := newTestServer(t)

	rec := ts.doMultipart(t, "file", "pic.png", pngBytes, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d: %s", rec.Code, rec.Body.String())
	}
	first := decodeJSON(t, rec)
	name, _ := first["filename"].(string)
	if name == "" || first["url"] != "/assets/"+name {
		t.Errorf("upload response = %v", first)
	}

	// Identical upload is idempotent: 200 with the existing URL.
	rec = ts.doMultipart(t, "file", "pic.png", pngBytes, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-upload = %d, want 200", rec.Code)
	}
	again := decodeJSON(t, rec)
	if again["url"] != first["url"] || again["filename"] != name {
		t.Errorf("re-upload response = %v, want same url/filename as %v", again, first)
	}

	// Different name, same content: a new immutable file.
	rec = ts.doMultipart(t, "file", "copy.png", pngBytes, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("renamed upload = %d, want 201", rec.Code)
	}
	renamed := decodeJSON(t, rec)
	if renamed["filename"] == name {
		t.Errorf("renamed upload deduped to %q, want a distinct file", renamed["filename"])
	}
}

func TestAssetUploadRejections(t *testing.T) {
	ts := newTestServer(t)

	svg := []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"><script>alert(1)</script></svg>")
	if rec := ts.doMultipart(t, "file", "evil.svg", svg, true); rec.Code != http.StatusBadRequest {
		t.Errorf("svg upload = %d, want 400", rec.Code)
	}
	if rec := ts.doMultipart(t, "file", "notes.txt", []byte("plain text"), true); rec.Code != http.StatusBadRequest {
		t.Errorf("txt upload = %d, want 400", rec.Code)
	}
	// PNG bytes claiming to be .jpg: mislabeled, rejected.
	if rec := ts.doMultipart(t, "file", "mislabeled.jpg", pngBytes, true); rec.Code != http.StatusBadRequest {
		t.Errorf("mislabeled upload = %d, want 400", rec.Code)
	}
	// Missing file field.
	if rec := ts.doMultipart(t, "other", "x.png", pngBytes, true); rec.Code != http.StatusBadRequest {
		t.Errorf("missing file field = %d, want 400", rec.Code)
	}
	if rec := ts.doMultipart(t, "file", "", pngBytes, true); rec.Code != http.StatusBadRequest {
		t.Errorf("non-file field = %d, want 400", rec.Code)
	}

	// Oversize: 10 MB + 1 byte of a PNG-prefixed payload.
	oversize := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{0}, assets.MaxSize)...)
	rec := ts.doMultipart(t, "file", "big.png", oversize, true)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize upload = %d, want 413", rec.Code)
	}
}

func TestAssetServing(t *testing.T) {
	ts := newTestServer(t)
	rec := ts.doMultipart(t, "file", "pic.png", pngBytes, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d: %s", rec.Code, rec.Body.String())
	}
	var up map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}

	// Public: no token, immutable cache, correct content type, exact bytes.
	req := httptest.NewRequest(http.MethodGet, up["url"], nil)
	served := httptest.NewRecorder()
	ts.e.ServeHTTP(served, req)
	if served.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", up["url"], served.Code)
	}
	if got := served.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}
	if got := served.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := served.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if !bytes.Equal(served.Body.Bytes(), pngBytes) {
		t.Error("served bytes differ from the upload")
	}
}

func TestAssetServingTraversalRejected(t *testing.T) {
	ts := newTestServer(t)
	if rec := ts.doMultipart(t, "file", "pic.png", pngBytes, true); rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d", rec.Code)
	}

	for _, target := range []string{
		"/assets/nope.png",         // never uploaded
		"/assets/..%2Fapi-token",   // encoded traversal toward the token file
		"/assets/%2e%2e%2fsecret",  // encoded traversal, no valid shape
		"/assets/../api-token",     // literal traversal
		"/assets/sub/dir/file.png", // subdirectory
		"/assets/.",                // current dir
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		ts.e.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
}

// The assets route namespace is reserved: no page can shadow it.
func TestAssetsRouteReserved(t *testing.T) {
	ts := newTestServer(t)
	rec := ts.do(t, http.MethodPost, "/api/pages", `{"route":"assets","markdown":"# mine"}`, true, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("publish at /assets = %d, want 400", rec.Code)
	}
	rec = ts.do(t, http.MethodGet, "/api/routes/assets/available", "", true, "")
	if decodeJSON(t, rec)["available"] != false {
		t.Error("route 'assets' reported available, want reserved")
	}
}
