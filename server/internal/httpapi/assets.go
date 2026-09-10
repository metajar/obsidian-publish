package httpapi

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"

	"obsidian-publish/server/internal/assets"
)

// assetURLPrefix is the public URL prefix under which assets are served.
const assetURLPrefix = "/assets/"

// uploadAsset handles POST /api/assets — multipart upload, form field "file".
// Images only (png/jpg/jpeg/gif/webp; SVG is rejected as an XSS vector), 10 MB
// max. Identical content dedupes to the existing file (200 instead of 201).
func (s *Server) uploadAsset(c echo.Context) error {
	if s.deps.Assets == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "asset storage is not configured")
	}
	mr, err := c.Request().MultipartReader()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "multipart/form-data body with a 'file' field is required")
	}
	var (
		content  []byte
		fileName string
	)
	for {
		part, perr := mr.NextPart()
		if errors.Is(perr, io.EOF) {
			break
		}
		if perr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "malformed multipart payload")
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_, _ = io.Copy(io.Discard, part) // drain so the reader can advance
			continue
		}
		// Read at most MaxSize+1 bytes so an oversize upload is detectable
		// without buffering it whole.
		content, err = io.ReadAll(io.LimitReader(part, assets.MaxSize+1))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "could not read the uploaded file")
		}
		fileName = part.FileName()
		break
	}
	if fileName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "multipart form field 'file' with an image file is required")
	}
	if len(content) > assets.MaxSize {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "asset exceeds the 10 MB size limit")
	}

	name, existed, err := s.deps.Assets.Save(content, fileName)
	if errors.Is(err, assets.ErrInvalidType) {
		return echo.NewHTTPError(http.StatusBadRequest, "only png, jpg, jpeg, gif, and webp images are allowed (svg and other formats are rejected)")
	}
	if err != nil {
		return err
	}
	s.logInfo("asset_upload", "filename", name, "deduped", existed)
	status := http.StatusCreated
	if existed {
		status = http.StatusOK
	}
	return c.JSON(status, map[string]string{
		"url":      assetURLPrefix + name,
		"filename": name,
	})
}

// serveAsset handles GET /assets/{filename} — public, immutable, images only.
// ValidFilename is the path-traversal guard: nothing outside the assets dir
// is ever opened.
func (s *Server) serveAsset(c echo.Context) error {
	if s.deps.Assets == nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	name := c.Param("filename")
	f, err := s.deps.Assets.Open(name)
	if errors.Is(err, assets.ErrNotFound) {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}

	// Content is addressed by hash and never modified, so readers may cache
	// it forever. The explicit Content-Type (set before ServeContent, which
	// respects it) covers webp, which Go's builtin extension table lacks.
	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	c.Response().Header().Set("Content-Type", assets.ContentType(name))
	http.ServeContent(c.Response(), c.Request(), filepath.Base(name), fi.ModTime(), f)
	return nil
}
