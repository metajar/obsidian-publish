// Package ratelimit provides a small in-memory sliding-window limiter for
// per-IP+route password attempts (PRD requirement S9).
package ratelimit

import (
	"sync"
	"time"
)

// Limiter limits events to max occurrences per window per (key1, key2) pair.
// It is safe for concurrent use. State is in-memory only — acceptable for the
// MVP threat model (deter casual brute force); restart resets the windows.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	events map[string][]time.Time
}

// New creates a Limiter allowing max events per window.
func New(max int, window time.Duration) *Limiter {
	return &Limiter{
		max:    max,
		window: window,
		events: make(map[string][]time.Time),
	}
}

// Allow records an attempt for (key1, key2) and reports whether it is within
// the limit. Time is injected for testability.
func (l *Limiter) Allow(key1, key2 string, now time.Time) bool {
	k := key1 + "|" + key2
	l.mu.Lock()
	defer l.mu.Unlock()

	times := l.events[k]
	kept := times[:0]
	cutoff := now.Add(-l.window)
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		if len(kept) == 0 {
			delete(l.events, k)
		} else {
			l.events[k] = kept
		}
		return false
	}
	l.events[k] = append(kept, now)
	if len(l.events) > 10_000 { // opportunistic pruning of fully-expired keys
		for key, ts := range l.events {
			if len(ts) == 0 || !ts[len(ts)-1].After(cutoff) {
				delete(l.events, key)
			}
		}
	}
	return true
}
