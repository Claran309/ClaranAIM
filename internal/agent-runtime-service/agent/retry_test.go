package agent

import (
	"context"
	"errors"
	"testing"
)

func TestIsRateLimitErrorRetries429AndQPM(t *testing.T) {
	cases := []string{
		"429 Too Many Requests",
		"openai: rate limit exceeded",
		"qpm limit reached, please slow down",
		"status code 429 from provider",
	}
	for _, message := range cases {
		if !isRateLimitError(context.Background(), errors.New(message)) {
			t.Fatalf("isRateLimitError(%q) = false, want true", message)
		}
	}
}

func TestIsRateLimitErrorDoesNotRetryPermanentErrors(t *testing.T) {
	cases := []string{
		"invalid api key",
		"model not found",
		"context deadline exceeded",
		"insufficient balance",
	}
	for _, message := range cases {
		if isRateLimitError(context.Background(), errors.New(message)) {
			t.Fatalf("isRateLimitError(%q) = true, want false", message)
		}
	}
	if isRateLimitError(context.Background(), nil) {
		t.Fatal("nil error must not be retryable")
	}
}
