// Package session issues and verifies short-lived, route-scoped,
// HMAC-signed session tokens delivered as HttpOnly cookies. A token is only
// valid for the exact route it was issued for and only until its expiry.
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CookieName is the single cookie name used for all route sessions; the
// route binding and expiry live inside the signed token value, and the
// cookie's Path is scoped to the issuing route.
const CookieName = "op_session"

// Manager signs and verifies session tokens with a server-side secret.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager creates a Manager. The secret must be non-empty; ttl is capped
// by the caller (the server uses 1 hour maximum).
func NewManager(secret []byte, ttl time.Duration) (*Manager, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("session: empty secret")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("session: non-positive ttl")
	}
	return &Manager{secret: secret, ttl: ttl}, nil
}

// TTL returns the session lifetime.
func (m *Manager) TTL() time.Duration { return m.ttl }

// Token returns a signed token granting access to route until expiry.
// payload = "<route>|<unix-expiry>"; token = b64url(payload) + "." + b64url(hmac).
func (m *Manager) Token(route string, now time.Time) string {
	payload := route + "|" + strconv.FormatInt(now.Add(m.ttl).Unix(), 10)
	sig := hmac.New(sha256.New, m.secret)
	sig.Write([]byte(payload))
	b64 := base64.RawURLEncoding
	return b64.EncodeToString([]byte(payload)) + "." + b64.EncodeToString(sig.Sum(nil))
}

// Verify reports whether token is a valid, unexpired grant for route.
func (m *Manager) Verify(route, token string, now time.Time) bool {
	payloadB64, sigB64, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return false
	}
	gotRoute, expStr, ok := strings.Cut(string(payload), "|")
	if !ok || gotRoute != route {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || now.Unix() >= exp {
		return false
	}
	return true
}

// Cookie returns the Set-Cookie for a successful auth on route: HttpOnly,
// Secure, scoped to the route path, expiring with the token.
func (m *Manager) Cookie(route string, now time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    m.Token(route, now),
		Path:     "/" + route,
		MaxAge:   int(m.ttl.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}
