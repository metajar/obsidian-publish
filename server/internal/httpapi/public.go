package httpapi

import (
	"errors"
	"html"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"obsidian-publish/server/internal/pwhash"
	"obsidian-publish/server/internal/session"
	"obsidian-publish/server/internal/store"
)

// servePage handles GET /{route} — the public reader plane.
//
// For a protected route with no valid session it serves ONLY the password
// form: no content, no title, no hash, no-cache headers on every response.
func (s *Server) servePage(c echo.Context) error {
	route := c.Param("route")
	page, err := s.deps.Store.Get(route)
	if errors.Is(err, store.ErrNotFound) {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return err
	}

	if page.PasswordHash != "" && !s.validSession(c, route) {
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.HTML(http.StatusOK, passwordForm(route, ""))
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	s.logInfo("page_serve", "route", route, "protected", page.PasswordHash != "")
	return c.HTML(http.StatusOK, page.ContentHTML)
}

// pageAuth handles POST /{route}/auth — public password verification.
// Rate-limited per IP+route. On success it sets the route-scoped session
// cookie and redirects to the page; on failure it re-serves the form (no
// information about the password beyond the generic error).
func (s *Server) pageAuth(c echo.Context) error {
	route := c.Param("route")
	page, err := s.deps.Store.Get(route)
	if errors.Is(err, store.ErrNotFound) {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return err
	}

	if page.PasswordHash == "" {
		// Not protected: nothing to verify, just send the reader to the page.
		return c.Redirect(http.StatusSeeOther, "/"+route)
	}

	ip := c.RealIP()
	now := time.Now()
	if !s.deps.Limiter.Allow(ip, route, now) {
		s.logInfo("auth_rate_limited", "route", route, "ip", ip)
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.HTML(http.StatusTooManyRequests, passwordForm(route, "Too many attempts. Try again in a minute."))
	}

	password := c.FormValue("password")
	ok, verr := pwhash.Verify(password, page.PasswordHash)
	if verr != nil || !ok {
		s.logInfo("auth_failure", "route", route, "ip", ip)
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.HTML(http.StatusUnauthorized, passwordForm(route, "Incorrect password."))
	}

	http.SetCookie(c.Response(), s.deps.Sessions.Cookie(route, now))
	s.logInfo("auth_success", "route", route, "ip", ip)
	return c.Redirect(http.StatusSeeOther, "/"+route)
}

// validSession checks the session cookie for an unexpired grant for route.
func (s *Server) validSession(c echo.Context, route string) bool {
	cookie, err := c.Cookie(session.CookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	return s.deps.Sessions.Verify(route, cookie.Value, time.Now())
}

// passwordForm renders the minimal password-entry page. It intentionally
// reveals nothing about the page beyond the fact that it is protected.
func passwordForm(route, errMsg string) string {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<p class="error">` + html.EscapeString(errMsg) + `</p>`
	}
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Password required</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  display: flex; min-height: 100vh; margin: 0; align-items: center; justify-content: center;
  background: #f5f5f5; color: #222; }
form { background: #fff; padding: 2rem; border-radius: 8px; box-shadow: 0 1px 4px rgba(0,0,0,.15);
  display: flex; flex-direction: column; gap: .75rem; width: min(20rem, 90vw); }
input { font-size: 1rem; padding: .5rem .75rem; border: 1px solid #bbb; border-radius: 4px; }
button { font-size: 1rem; padding: .5rem .75rem; border: 0; border-radius: 4px;
  background: #222; color: #fff; cursor: pointer; }
.error { color: #b00020; margin: 0; font-size: .9rem; }
</style>
</head>
<body>
<form method="post" action="/` + html.EscapeString(route) + `/auth">
<p style="margin:0">This page is password protected.</p>
` + errHTML + `
<input type="password" name="password" placeholder="Password" autofocus autocomplete="current-password" required>
<button type="submit">Unlock</button>
</form>
</body>
</html>
`
}
