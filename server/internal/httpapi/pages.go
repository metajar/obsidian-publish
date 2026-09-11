package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"obsidian-publish/server/internal/markdown"
	"obsidian-publish/server/internal/pwhash"
	"obsidian-publish/server/internal/store"
)

// timeFormat is the API timestamp format (RFC3339, UTC seconds precision).
const timeFormat = time.RFC3339

// createPageRequest is the POST /api/pages payload.
type createPageRequest struct {
	Route        string  `json:"route"`
	Title        string  `json:"title"`
	Markdown     string  `json:"markdown"`
	Password     *string `json:"password"` // nil or "" = unprotected
	SourceNoteID string  `json:"source_note_id"`
	ThemeCSS     string  `json:"theme_css"` // optional; non-empty overrides the global theme
}

// updatePageRequest is the PUT /api/pages/{route} payload. Absent fields are
// left unchanged; an explicit JSON null password removes protection.
type updatePageRequest struct {
	Title        *string        `json:"title"`
	Markdown     *string        `json:"markdown"`
	Password     OptionalString `json:"password"`
	SourceNoteID OptionalString `json:"source_note_id"`
	ThemeCSS     OptionalString `json:"theme_css"` // absent = keep; null or "" = clear override
}

// pageResponse is the API projection of a page. It never contains the
// password hash, page content, or theme CSS.
type pageResponse struct {
	Route             string `json:"route"`
	Title             string `json:"title"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	PasswordProtected bool   `json:"password_protected"`
	SourceNoteID      string `json:"source_note_id,omitempty"`
	ThemeOverride     bool   `json:"theme_override"`
	URL               string `json:"url"`
}

// OptionalString distinguishes "field absent in JSON" from "field present
// but null". Absent → Present=false; null → Present=true, Value=nil;
// string → Present=true, Value=&s.
type OptionalString struct {
	Present bool
	Value   *string
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *OptionalString) UnmarshalJSON(b []byte) error {
	o.Present = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}

// routeAvailable handles GET /api/routes/{route}/available.
func (s *Server) routeAvailable(c echo.Context) error {
	route := c.Param("route")
	available := store.ValidRoute(route)
	if available {
		_, err := s.deps.Store.Get(route)
		available = errors.Is(err, store.ErrNotFound)
	}
	return c.JSON(http.StatusOK, map[string]bool{"available": available})
}

// createPage handles POST /api/pages.
func (s *Server) createPage(c echo.Context) error {
	var req createPageRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON payload")
	}
	if req.Route == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "route is required")
	}
	if !store.ValidRoute(req.Route) {
		return echo.NewHTTPError(http.StatusBadRequest, "route must be a URL-safe slug: 1-64 chars of lowercase letters, digits, hyphens; must start and end alphanumeric; some names are reserved")
	}
	if len(req.ThemeCSS) > maxThemeCSS {
		return echo.NewHTTPError(http.StatusBadRequest, "theme_css exceeds the 256 KB theme size limit")
	}
	if _, err := s.deps.Store.Get(req.Route); err == nil {
		return echo.NewHTTPError(http.StatusConflict, "route already taken")
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	title := req.Title
	if title == "" {
		title = req.Route
	}
	passwordHash := ""
	if req.Password != nil && *req.Password != "" {
		hash, err := pwhash.Hash(*req.Password)
		if err != nil {
			return err
		}
		passwordHash = hash
	}
	html, err := markdown.Document(title, req.Markdown, s.effectiveCSS(req.ThemeCSS))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "markdown could not be rendered")
	}
	page := store.Page{
		Route:        req.Route,
		Title:        title,
		ContentMD:    req.Markdown,
		ContentHTML:  html,
		PasswordHash: passwordHash,
		SourceNoteID: req.SourceNoteID,
		ThemeCSS:     req.ThemeCSS,
	}
	if err := s.deps.Store.Create(page); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return echo.NewHTTPError(http.StatusConflict, "route already taken")
		}
		return err
	}
	stored, err := s.deps.Store.Get(req.Route)
	if err != nil {
		return err
	}
	s.logInfo("publish", "route", req.Route, "protected", passwordHash != "")
	return c.JSON(http.StatusCreated, s.pageResponse(c, stored))
}

// listPages handles GET /api/pages.
func (s *Server) listPages(c echo.Context) error {
	pages, err := s.deps.Store.List()
	if err != nil {
		return err
	}
	if pages == nil {
		pages = []store.PageMeta{}
	}
	return c.JSON(http.StatusOK, pages)
}

// updatePage handles PUT /api/pages/{route}.
func (s *Server) updatePage(c echo.Context) error {
	route := c.Param("route")
	var req updatePageRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON payload")
	}
	page, err := s.deps.Store.Get(route)
	if errors.Is(err, store.ErrNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "no page published at this route")
	}
	if err != nil {
		return err
	}

	title := page.Title
	if req.Title != nil {
		title = *req.Title
	}
	themeCSS := page.ThemeCSS
	if req.ThemeCSS.Present {
		switch {
		case req.ThemeCSS.Value == nil || *req.ThemeCSS.Value == "":
			themeCSS = "" // explicit null (or empty string) clears the override
		case len(*req.ThemeCSS.Value) > maxThemeCSS:
			return echo.NewHTTPError(http.StatusBadRequest, "theme_css exceeds the 256 KB theme size limit")
		default:
			themeCSS = *req.ThemeCSS.Value
		}
	}
	contentMD := page.ContentMD
	contentHTML := page.ContentHTML
	if req.Markdown != nil || (req.Title != nil && title != page.Title) || themeCSS != page.ThemeCSS {
		// Content, title, or theme override changed: re-render with the
		// effective stylesheet (per-page override > global theme > default).
		if req.Markdown != nil {
			contentMD = *req.Markdown
		}
		html, rerr := markdown.Document(title, contentMD, s.effectiveCSS(themeCSS))
		if rerr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "markdown could not be rendered")
		}
		contentHTML = html
	}
	passwordHash := page.PasswordHash
	switch {
	case req.Password.Present && (req.Password.Value == nil || *req.Password.Value == ""):
		passwordHash = "" // explicit null (or empty string) removes protection
	case req.Password.Present:
		hash, herr := pwhash.Hash(*req.Password.Value)
		if herr != nil {
			return herr
		}
		passwordHash = hash
	}
	sourceNoteID := page.SourceNoteID
	if req.SourceNoteID.Present {
		if req.SourceNoteID.Value != nil {
			sourceNoteID = *req.SourceNoteID.Value
		} else {
			sourceNoteID = ""
		}
	}

	updated, err := s.deps.Store.Update(route, title, contentMD, contentHTML, passwordHash, sourceNoteID, themeCSS)
	if errors.Is(err, store.ErrNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "no page published at this route")
	}
	if err != nil {
		return err
	}
	s.logInfo("update", "route", route, "protected", passwordHash != "")
	return c.JSON(http.StatusOK, s.pageResponse(c, updated))
}

// deletePage handles DELETE /api/pages/{route}.
func (s *Server) deletePage(c echo.Context) error {
	route := c.Param("route")
	err := s.deps.Store.Delete(route)
	if errors.Is(err, store.ErrNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "no page published at this route")
	}
	if err != nil {
		return err
	}
	s.logInfo("unpublish", "route", route)
	// 200 with a JSON body, not 204: Obsidian's requestUrl parses every
	// response body as JSON and rejects an empty one ("Unexpected end of
	// JSON input"), which the plugin would misreport as an unreachable
	// server even though the delete succeeded.
	return c.JSON(http.StatusOK, map[string]string{"deleted": route})
}

func (s *Server) pageResponse(c echo.Context, p *store.Page) pageResponse {
	return pageResponse{
		Route:             p.Route,
		Title:             p.Title,
		CreatedAt:         p.CreatedAt.Format(timeFormat),
		UpdatedAt:         p.UpdatedAt.Format(timeFormat),
		PasswordProtected: p.PasswordHash != "",
		SourceNoteID:      p.SourceNoteID,
		ThemeOverride:     p.ThemeCSS != "",
		URL:               s.liveURL(c, p.Route),
	}
}

func (s *Server) liveURL(c echo.Context, route string) string {
	if s.deps.BaseURL != "" {
		return s.deps.BaseURL + "/" + route
	}
	return c.Scheme() + "://" + c.Request().Host + "/" + route
}

func (s *Server) logInfo(msg string, args ...any) {
	if s.deps.Log != nil {
		s.deps.Log.Info(msg, args...)
	}
}
