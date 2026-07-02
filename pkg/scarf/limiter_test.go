package scarf

import (
	"testing"
	"time"
)

func Test_UnitLimiterRateLimit(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	l := newLimiter(time.Hour, 1000)
	l.now = func() time.Time { return now }

	if !l.allow("c1") {
		t.Error("first allow(c1) should be true")
	}
	if l.allow("c1") {
		t.Error("immediate second allow(c1) should be false (rate-limited)")
	}

	now = now.Add(2 * time.Hour)
	if !l.allow("c1") {
		t.Error("allow(c1) after window should be true")
	}
}

func Test_UnitLimiterNoWindow(t *testing.T) {
	l := newLimiter(0, 0)

	if !l.allow("c1") {
		t.Error("allow(c1) with window=0 should be true")
	}
	if !l.allow("c1") {
		t.Error("second allow(c1) with window=0 should be true")
	}

	if l.sent.Load() != 2 {
		t.Errorf("sent counter = %d, want 2", l.sent.Load())
	}
	if l.day.Estimate() == 0 {
		t.Error("day.Estimate() should be > 0 (cardinality tracked even with window=0)")
	}
}

func Test_UnitLimiterEmptyKey(t *testing.T) {
	l := newLimiter(time.Hour, 1000)

	if !l.allow("") {
		t.Error("allow(\"\") should be true")
	}
	if !l.allow("") {
		t.Error("second allow(\"\") should be true (not rate-limited)")
	}

	if l.anonSent.Load() != 2 {
		t.Errorf("anonSent = %d, want 2", l.anonSent.Load())
	}
	if l.day.Estimate() != 0 {
		t.Errorf("day.Estimate() = %d, want 0 (empty keys not counted)", l.day.Estimate())
	}
}

func Test_UnitLimiterIndependentKeys(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	l := newLimiter(time.Hour, 1000)
	l.now = func() time.Time { return now }

	if !l.allow("c1") {
		t.Error("first allow(c1) should be true")
	}
	if l.allow("c1") {
		t.Error("second allow(c1) should be false")
	}
	if !l.allow("c2") {
		t.Error("allow(c2) should be true (independent from c1)")
	}
}

func Test_UnitLimiterEviction(t *testing.T) {
	l := newLimiter(time.Hour, 1)

	if !l.allow("a") {
		t.Error("allow(a) should be true")
	}
	if !l.allow("b") {
		t.Error("allow(b) should be true (evicts a)")
	}

	if l.evictions.Load() != 1 {
		t.Errorf("evictions = %d, want 1", l.evictions.Load())
	}

	if !l.allow("a") {
		t.Error("allow(a) should be true again (was evicted)")
	}
}

func Test_UnitLimiterHLLAccuracy(t *testing.T) {
	l := newLimiter(0, 0)

	const count = 1000
	for i := 0; i < count; i++ {
		l.allow(string(rune('a'+i%26)) + string(rune('0'+i/26)))
	}

	estimate := l.day.Estimate()
	lower := uint64(float64(count) * 0.95)
	upper := uint64(float64(count) * 1.05)
	if estimate < lower || estimate > upper {
		t.Errorf("day.Estimate() = %d, want within [%d, %d] (±5%% of %d)", estimate, lower, upper, count)
	}

	totalEstimate := l.total.Estimate()
	if totalEstimate < lower || totalEstimate > upper {
		t.Errorf("total.Estimate() = %d, want within [%d, %d] (±5%% of %d)", totalEstimate, lower, upper, count)
	}
}

func Test_UnitLimiterRollover(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	l := newLimiter(0, 0)
	l.now = func() time.Time { return now }

	l.allow("c1")
	l.allow("c2")

	totalBefore := l.total.Estimate()

	now = now.Add(24 * time.Hour)
	l.report()

	dayAfter := l.day.Estimate()
	totalAfter := l.total.Estimate()

	if dayAfter != 0 {
		t.Errorf("day.Estimate() after rollover = %d, want 0", dayAfter)
	}
	if totalAfter < totalBefore {
		t.Errorf("total.Estimate() after rollover = %d, should be >= %d (total persists)", totalAfter, totalBefore)
	}
}

func Test_UnitLimiterCounters(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	l := newLimiter(time.Hour, 1000)
	l.now = func() time.Time { return now }

	l.allow("c1")
	l.allow("c1")
	l.allow("c2")
	l.allow("")
	l.allow("")

	if got := l.sent.Load(); got != 2 {
		t.Errorf("sent = %d, want 2 (c1 first time, c2 first time)", got)
	}
	if got := l.suppressed.Load(); got != 1 {
		t.Errorf("suppressed = %d, want 1 (c1 second time)", got)
	}
	if got := l.anonSent.Load(); got != 2 {
		t.Errorf("anonSent = %d, want 2 (empty keys)", got)
	}
}
