package middleware

import (
	"testing"
	"time"
)

func TestTokenBucketLimiterEnforcesCapacityAndIsolatesKeys(t *testing.T) {
	limiter := newTokenBucketLimiter(2, time.Hour)
	if !limiter.allow("user:1") || !limiter.allow("user:1") {
		t.Fatal("first two requests should be allowed")
	}
	if limiter.allow("user:1") {
		t.Fatal("third request in the same window should be rejected")
	}
	if !limiter.allow("user:2") {
		t.Fatal("other user should have an independent bucket")
	}
}

func TestTokenBucketLimiterRefillsAfterInterval(t *testing.T) {
	limiter := newTokenBucketLimiter(1, 10*time.Millisecond)
	if !limiter.allow("ip:1.2.3.4") {
		t.Fatal("first request should be allowed")
	}
	if limiter.allow("ip:1.2.3.4") {
		t.Fatal("second request before refill should be rejected")
	}
	time.Sleep(15 * time.Millisecond)
	if !limiter.allow("ip:1.2.3.4") {
		t.Fatal("request after refill interval should be allowed")
	}
}

func TestTokenBucketLimiterUsesSafeDefaults(t *testing.T) {
	limiter := newTokenBucketLimiter(0, 0)
	if limiter.capacity != 120 || limiter.refillInterval != time.Minute {
		t.Fatalf("defaults = (%d, %v), want (120, 1m)", limiter.capacity, limiter.refillInterval)
	}
}
