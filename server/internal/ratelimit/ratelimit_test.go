package ratelimit

import (
	"testing"
	"time"
)

func TestAllowWithinLimit(t *testing.T) {
	l := New(5, time.Minute)
	now := time.Now()
	for i := 0; i < 5; i++ {
		if !l.Allow("1.2.3.4", "note-a", now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("attempt %d denied, want allowed", i+1)
		}
	}
	if l.Allow("1.2.3.4", "note-a", now.Add(5*time.Second)) {
		t.Error("6th attempt within window allowed, want denied")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := New(2, time.Minute)
	now := time.Now()
	l.Allow("1.1.1.1", "note-a", now)
	l.Allow("1.1.1.1", "note-a", now)
	if l.Allow("1.1.1.1", "note-a", now) {
		t.Error("same key over limit allowed")
	}
	if !l.Allow("1.1.1.1", "note-b", now) {
		t.Error("different route denied under its own limit")
	}
	if !l.Allow("2.2.2.2", "note-a", now) {
		t.Error("different IP denied under its own limit")
	}
}

func TestWindowSlides(t *testing.T) {
	l := New(2, time.Minute)
	now := time.Now()
	l.Allow("ip", "route", now)
	l.Allow("ip", "route", now.Add(10*time.Second))
	if l.Allow("ip", "route", now.Add(20*time.Second)) {
		t.Fatal("third attempt within window allowed")
	}
	// After the first two attempts fall out of the window, allow again.
	if !l.Allow("ip", "route", now.Add(time.Minute+11*time.Second)) {
		t.Error("attempt after window expired denied")
	}
}
