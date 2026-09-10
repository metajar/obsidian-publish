package session

import (
	"testing"
	"time"
)

func newManager(t *testing.T, ttl time.Duration) *Manager {
	t.Helper()
	m, err := NewManager([]byte("0123456789abcdef0123456789abcdef"), ttl)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func TestSignVerifyRoundtrip(t *testing.T) {
	m := newManager(t, time.Hour)
	now := time.Now()
	tok := m.Token("my-note", now)
	if !m.Verify("my-note", tok, now.Add(time.Minute)) {
		t.Error("valid token failed verification")
	}
}

func TestVerifyWrongRoute(t *testing.T) {
	m := newManager(t, time.Hour)
	now := time.Now()
	tok := m.Token("my-note", now)
	if m.Verify("other-note", tok, now) {
		t.Error("token verified for a different route")
	}
}

func TestVerifyExpired(t *testing.T) {
	m := newManager(t, time.Hour)
	now := time.Now()
	tok := m.Token("my-note", now)
	if m.Verify("my-note", tok, now.Add(time.Hour+time.Second)) {
		t.Error("expired token verified")
	}
	// Exactly at expiry it must also fail.
	if m.Verify("my-note", tok, now.Add(time.Hour)) {
		t.Error("token verified at its exact expiry instant")
	}
}

func TestVerifyTampered(t *testing.T) {
	m := newManager(t, time.Hour)
	now := time.Now()
	tok := m.Token("my-note", now)

	// Flip a character in the payload half.
	b := []byte(tok)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	if m.Verify("my-note", string(b), now) {
		t.Error("tampered token verified")
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	m1 := newManager(t, time.Hour)
	m2, err := NewManager([]byte("ffffffffffffffffffffffffffffffff"), time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	tok := m1.Token("my-note", time.Now())
	if m2.Verify("my-note", tok, time.Now()) {
		t.Error("token signed with a different secret verified")
	}
}

func TestVerifyGarbage(t *testing.T) {
	m := newManager(t, time.Hour)
	for _, tok := range []string{"", "abc", "a.b", "....", "AAAA.BBBB"} {
		if m.Verify("my-note", tok, time.Now()) {
			t.Errorf("garbage token %q verified", tok)
		}
	}
}

func TestCookieAttributes(t *testing.T) {
	m := newManager(t, 30*time.Minute)
	ck := m.Cookie("my-note", time.Now())
	if !ck.HttpOnly {
		t.Error("cookie not HttpOnly")
	}
	if !ck.Secure {
		t.Error("cookie not Secure")
	}
	if ck.Path != "/my-note" {
		t.Errorf("cookie Path = %q, want /my-note", ck.Path)
	}
	if ck.Name != CookieName {
		t.Errorf("cookie Name = %q, want %q", ck.Name, CookieName)
	}
	if ck.MaxAge != int((30 * time.Minute).Seconds()) {
		t.Errorf("cookie MaxAge = %d, want 1800", ck.MaxAge)
	}
}
