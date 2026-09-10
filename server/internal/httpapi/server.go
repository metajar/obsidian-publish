// Package httpapi wires the Echo router: the bearer-token-authenticated
// /api/* plane (owner/plugin) and the fully separate public reader plane
// (GET /{route}, POST /{route}/auth). The two planes never mix.
package httpapi

import (
	"crypto/hmac"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"obsidian-publish/server/internal/ratelimit"
	"obsidian-publish/server/internal/session"
	"obsidian-publish/server/internal/store"
)

// Deps carries everything the handlers need.
type Deps struct {
	Store    *store.Store
	Token    string // API bearer token
	Sessions *session.Manager
	Limiter  *ratelimit.Limiter
	BaseURL  string // optional; empty = derive live URLs from request Host
	Log      *slog.Logger
}

// Server holds handler state.
type Server struct {
	deps Deps
}

// New builds the Echo instance with all routes registered.
func New(d Deps) (*echo.Echo, *Server) {
	s := &Server{deps: d}
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = errorHandler

	// Owner plane: every /api/* call requires the bearer token.
	api := e.Group("/api", bearerAuth(d.Token))
	api.GET("/routes/:route/available", s.routeAvailable)
	api.POST("/pages", s.createPage)
	api.GET("/pages", s.listPages)
	api.PUT("/pages/:route", s.updatePage)
	api.DELETE("/pages/:route", s.deletePage)

	// Public reader plane: no token, page password gate only.
	e.GET("/:route", s.servePage)
	e.POST("/:route/auth", s.pageAuth)

	return e, s
}

// bearerAuth rejects any request without an exactly-matching bearer token.
// Comparison is constant-time; malformed or missing headers fail closed.
func bearerAuth(token string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			scheme, rest, found := strings.Cut(header, " ")
			if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(rest) == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing or malformed Authorization header")
			}
			if !hmac.Equal([]byte(strings.TrimSpace(rest)), []byte(token)) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}
			return next(c)
		}
	}
}

// errorHandler keeps API errors as JSON and public errors as minimal HTML,
// with no caching of either.
func errorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	var he *echo.HTTPError
	if !errors.As(err, &he) {
		he = echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(c.Request().URL.Path, "/api") {
		msg := http.StatusText(he.Code)
		if m, ok := he.Message.(string); ok && m != "" {
			msg = m
		}
		_ = c.JSON(he.Code, map[string]string{"error": msg})
		return
	}
	_ = c.HTML(he.Code, minimalStatusPage(he.Code))
}

// minimalStatusPage renders a small, content-free status page. It must never
// leak page content or hashes.
func minimalStatusPage(code int) string {
	return `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>` + strconv.Itoa(code) + `</title></head>
<body style="font-family:sans-serif;text-align:center;padding:4rem 1rem;">
<p>` + strconv.Itoa(code) + ` — ` + html.EscapeString(http.StatusText(code)) + `</p>
</body>
</html>
`
}
