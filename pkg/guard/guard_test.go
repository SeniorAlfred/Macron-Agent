package guard

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToTheLimit(t *testing.T) {
	r := NewRateLimiter(2, time.Minute)
	for i := 0; i < 2; i++ {
		if ok, _ := r.Allow("user"); !ok {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}
	ok, retry := r.Allow("user")
	if ok {
		t.Fatal("the third call in the window must be refused")
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("the retry delay should be inside the window, got %v", retry)
	}
	if ok, _ := r.Allow("other"); !ok {
		t.Fatal("another key has its own budget")
	}
}

func TestRateLimiterNilAndOpenAreHarmless(t *testing.T) {
	var nilLimiter *RateLimiter
	if ok, _ := nilLimiter.Allow("x"); !ok {
		t.Fatal("a nil limiter must allow everything")
	}
	if ok, _ := NewRateLimiter(0, time.Minute).Allow("x"); !ok {
		t.Fatal("a zero limit means no limit")
	}
}

func TestRateLimiterFreesOldHits(t *testing.T) {
	r := NewRateLimiter(1, 20*time.Millisecond)
	if ok, _ := r.Allow("k"); !ok {
		t.Fatal("the first call should pass")
	}
	if ok, _ := r.Allow("k"); ok {
		t.Fatal("the second call should be refused")
	}
	time.Sleep(30 * time.Millisecond)
	if ok, _ := r.Allow("k"); !ok {
		t.Fatal("the window should have passed")
	}
}

func TestGateLimitsConcurrency(t *testing.T) {
	g := NewGate(1)
	release, why := g.TryAcquire("first")
	if release == nil {
		t.Fatalf("the first acquisition should work: %v", why)
	}
	if release2, _ := g.TryAcquire("second"); release2 != nil {
		t.Fatal("a full gate must not hand out a second slot")
	}
	release()
	if release3, _ := g.TryAcquire("second"); release3 == nil {
		t.Fatal("the slot should be free again after the release")
	} else {
		release3()
	}
}
