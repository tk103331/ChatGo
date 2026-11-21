package relay

import (
	"sync"
	"time"
)

// rateLimiter is a small fixed-window limiter for pairing attempts. It is not
// meant to replace a fronting proxy's DDoS protection; it only stops a single
// source from grinding pair codes.
type rateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	limit   int
	entries map[string]*rateEntry
}

type rateEntry struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, entries: map[string]*rateEntry{}}
}

func (l *rateLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[key]
	if e == nil || now.Sub(e.start) > l.window {
		l.entries[key] = &rateEntry{start: now, count: 1}
		return true
	}
	e.count++
	if e.count > l.limit {
		return false
	}
	return true
}

func (l *rateLimiter) sweep(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, e := range l.entries {
		if now.Sub(e.start) > 2*l.window {
			delete(l.entries, k)
		}
	}
}
