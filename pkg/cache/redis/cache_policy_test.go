package redis

import (
	"testing"
	"time"
)

func TestJitterExpirationStaysInExpectedRange(t *testing.T) {
	base := 10 * time.Minute
	jitter := time.Minute
	for i := 0; i < 100; i++ {
		got := jitterExpiration(base, jitter)
		if got < 9*time.Minute || got > 11*time.Minute {
			t.Fatalf("expiration out of range: %s", got)
		}
	}
}

func TestIsCachedNull(t *testing.T) {
	if !isCachedNull(cacheNullValue) {
		t.Fatal("expected null marker to be recognized")
	}
	if isCachedNull(`{"id":1}`) {
		t.Fatal("normal json must not be recognized as null marker")
	}
}
