package httpapi

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"obsidian-publish/server/internal/markdown"
)

// Theme size bounds. Arbitrary CSS is allowed (single-owner, self-hosted —
// a decided product call), only its size is capped.
const (
	maxThemeCSS     = 256 << 10 // 256 KB
	maxThemeNameLen = 200
)

// themeResponse is the GET/POST /api/theme payload.
type themeResponse struct {
	CSS       string `json:"css"`
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
}

// setThemeRequest is the POST /api/theme payload. An absent or empty css
// resets the site to the built-in default styling; name is optional.
type setThemeRequest struct {
	CSS  string `json:"css"`
	Name string `json:"name"`
}

// getTheme handles GET /api/theme.
func (s *Server) getTheme(c echo.Context) error {
	theme, err := s.deps.Store.GetTheme()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, themeResponse{
		CSS:       theme.CSS,
		Name:      theme.Name,
		UpdatedAt: theme.UpdatedAt.Format(timeFormat),
	})
}

// setTheme handles POST /api/theme. Setting a theme re-renders every page
// that does not carry its own theme_css override, so the change takes effect
// immediately; override pages are untouched.
func (s *Server) setTheme(c echo.Context) error {
	var req setThemeRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON payload")
	}
	if len(req.CSS) > maxThemeCSS {
		return echo.NewHTTPError(http.StatusBadRequest, "css exceeds the 256 KB theme size limit")
	}
	if len(req.Name) > maxThemeNameLen {
		return echo.NewHTTPError(http.StatusBadRequest, "name exceeds 200 characters")
	}
	theme, err := s.deps.Store.SetTheme(req.CSS, req.Name)
	if err != nil {
		return err
	}
	s.rerenderThemedPages(theme.CSS)
	s.logInfo("theme_set", "name", theme.Name, "size", len(theme.CSS))
	return c.JSON(http.StatusOK, themeResponse{
		CSS:       theme.CSS,
		Name:      theme.Name,
		UpdatedAt: theme.UpdatedAt.Format(timeFormat),
	})
}

// rerenderThemedPages re-renders all pages without a per-page theme_css using
// the global css ("" = built-in default). Pages with their own override keep
// their existing render. Failures are logged and skipped — one bad page must
// not block a theme change.
func (s *Server) rerenderThemedPages(css string) {
	metas, err := s.deps.Store.List()
	if err != nil {
		s.logInfo("theme_rerender_error", "err", err.Error())
		return
	}
	for _, m := range metas {
		if m.ThemeOverride {
			continue
		}
		page, err := s.deps.Store.Get(m.Route)
		if err != nil {
			s.logInfo("theme_rerender_error", "route", m.Route, "err", err.Error())
			continue
		}
		html, err := markdown.Document(page.Title, page.ContentMD, css)
		if err != nil {
			s.logInfo("theme_rerender_error", "route", m.Route, "err", err.Error())
			continue
		}
		if err := s.deps.Store.SetRendered(page.Route, html); err != nil {
			s.logInfo("theme_rerender_error", "route", m.Route, "err", err.Error())
		}
	}
}

// effectiveCSS resolves the stylesheet precedence for a page render:
// per-page theme_css > global theme css > built-in default ("" signals the
// default to markdown.Document).
func (s *Server) effectiveCSS(pageThemeCSS string) string {
	if pageThemeCSS != "" {
		return pageThemeCSS
	}
	theme, err := s.deps.Store.GetTheme()
	if err != nil {
		return "" // fall back to the built-in default stylesheet
	}
	return theme.CSS
}
