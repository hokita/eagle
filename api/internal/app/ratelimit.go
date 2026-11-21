package app

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	hits   []time.Time
}

func NewRateLimiter(limit int, window time.Duration, now func() time.Time) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, now: now}
}

func (l *RateLimiter) Take() (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)
	i := 0
	for i < len(l.hits) && !l.hits[i].After(cutoff) {
		i++
	}
	l.hits = l.hits[i:]

	if len(l.hits) >= l.limit {
		return false, l.hits[0].Add(l.window).Sub(now)
	}
	l.hits = append(l.hits, now)
	return true, 0
}
