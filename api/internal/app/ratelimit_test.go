package app

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiterWindow(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := t0
	l := NewRateLimiter(2, time.Hour, func() time.Time { return now })

	for i := 0; i < 2; i++ {
		if ok, _ := l.Take(); !ok {
			t.Fatalf("take %d: want ok", i)
		}
	}
	ok, retry := l.Take()
	if ok || retry != time.Hour {
		t.Fatalf("third take: ok=%v retry=%v, want false, 1h", ok, retry)
	}

	now = t0.Add(30 * time.Minute)
	ok, retry = l.Take()
	if ok || retry != 30*time.Minute {
		t.Fatalf("at 30m: ok=%v retry=%v, want false, 30m", ok, retry)
	}

	now = t0.Add(time.Hour)
	if ok, _ := l.Take(); !ok {
		t.Fatal("at 1h: want ok")
	}
}

func TestRateLimiterConcurrent(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewRateLimiter(50, time.Hour, func() time.Time { return t0 })

	var okCount atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Take(); ok {
				okCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := okCount.Load(); got != 50 {
		t.Fatalf("ok takes = %d, want 50", got)
	}
}
