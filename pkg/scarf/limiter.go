package scarf

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/axiomhq/hyperloglog"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultCacheSize is the default max number of distinct cluster IDs tracked.
	DefaultCacheSize = 550000
	reportInterval   = time.Hour
)

type limiter struct {
	mu       sync.Mutex
	window   time.Duration
	cache    *simplelru.LRU[string, time.Time]
	capacity int
	day      *hyperloglog.Sketch
	total    *hyperloglog.Sketch
	dayKey   int
	now      func() time.Time

	sent       atomic.Uint64
	suppressed atomic.Uint64
	anonSent   atomic.Uint64
	evictions  atomic.Uint64
}

func newLimiter(window time.Duration, size int) *limiter {
	l := &limiter{
		window: window,
		day:    hyperloglog.New14(),
		total:  hyperloglog.New14(),
		now:    time.Now,
	}
	l.dayKey = dayKey(l.now())

	if window > 0 {
		if size <= 0 {
			size = DefaultCacheSize
		}
		l.capacity = size
		cache, err := simplelru.NewLRU(size, func(_ string, _ time.Time) {
			l.evictions.Add(1)
		})
		if err != nil {
			logrus.Errorf("failed to create rate-limit cache: %v", err)
		} else {
			l.cache = cache
		}
	}
	return l
}

func dayKey(t time.Time) int {
	return t.Year()*1000 + t.YearDay()
}

func (l *limiter) allow(key string) bool {
	if key == "" {
		l.anonSent.Add(1)
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.rolloverLocked(now)
	kb := []byte(key)
	l.day.Insert(kb)
	l.total.Insert(kb)

	if l.window <= 0 || l.cache == nil {
		l.sent.Add(1)
		return true
	}

	if last, ok := l.cache.Get(key); ok && now.Sub(last) < l.window {
		l.suppressed.Add(1)
		return false
	}

	l.cache.Add(key, now)
	l.sent.Add(1)
	return true
}

func (l *limiter) rolloverLocked(now time.Time) {
	k := dayKey(now)
	if k == l.dayKey {
		return
	}
	logrus.WithField("distinct_day", l.day.Estimate()).Info("scarf daily rollover")
	l.day = hyperloglog.New14()
	l.dayKey = k
}

func (l *limiter) report() {
	l.mu.Lock()
	l.rolloverLocked(l.now())
	distinctDay := l.day.Estimate()
	distinctTotal := l.total.Estimate()
	cacheEntries := 0
	if l.cache != nil {
		cacheEntries = l.cache.Len()
	}
	l.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"distinct_day":   distinctDay,
		"distinct_total": distinctTotal,
		"cache_entries":  cacheEntries,
		"cache_cap":      l.capacity,
		"evictions":      l.evictions.Load(),
		"sent":           l.sent.Load(),
		"suppressed":     l.suppressed.Load(),
		"anon_sent":      l.anonSent.Load(),
	}).Info("scarf rate-limit stats")
}

func (l *limiter) reportLoop(ctx context.Context) {
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.report()
		}
	}
}
