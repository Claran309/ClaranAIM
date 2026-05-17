package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestTokenBucketAllowsBurstThenRejectsUntilRefill(t *testing.T) {
	limiter := newTokenBucketLimiter(2, time.Second)

	if !limiter.allow("user:1") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.allow("user:1") {
		t.Fatal("second request should be allowed")
	}
	if limiter.allow("user:1") {
		t.Fatal("third request should be rejected before refill")
	}
}

func TestRateLimitKeyPrefersAuthenticatedUser(t *testing.T) {
	var c app.RequestContext
	c.Set("userID", int64(1000000001))

	key := rateLimitKey(context.Background(), &c)

	if key != "user:1000000001" {
		t.Fatalf("key = %q, want user:1000000001", key)
	}
}
