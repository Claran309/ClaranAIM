package cache

import (
	"testing"
	"time"
)

func TestPolicyKeyAndJitter(t *testing.T) {
	policy := UserInfoPolicy(1001)
	if policy.Key != "user:info:1001" {
		t.Fatalf("key = %q, want user:info:1001", policy.Key)
	}
	if policy.TTL != 15*time.Minute {
		t.Fatalf("ttl = %s, want 15m", policy.TTL)
	}
	if policy.Jitter <= 0 {
		t.Fatalf("jitter = %s, want positive", policy.Jitter)
	}
}

func TestKeysForConversationParticipants(t *testing.T) {
	keys := ConversationListKeys([]int64{3, 1, 3})
	want := []string{"user:conversations:3", "user:conversations:1"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys[%d] = %q, want %q", i, keys[i], want[i])
		}
	}
}
