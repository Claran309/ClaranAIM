package component

import (
	"fmt"
	"testing"
)

// TestStoreEvictsSessionCacheBeyondLimit 验证会话缓存有上限，且淘汰后仍能从磁盘恢复。
func TestStoreEvictsSessionCacheBeyondLimit(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	for i := 0; i < maxCachedSessions+20; i++ {
		if _, err := store.GetSession(fmt.Sprintf("session-%03d", i)); err != nil {
			t.Fatalf("GetSession(%d) error = %v", i, err)
		}
	}
	store.mu.Lock()
	cached := len(store.sessions)
	store.mu.Unlock()
	if cached > maxCachedSessions {
		t.Fatalf("cached sessions = %d, want <= %d", cached, maxCachedSessions)
	}

	// 被淘汰的会话必须能从 JSONL 重新加载。
	if _, err := store.GetSession("session-000"); err != nil {
		t.Fatalf("reload evicted session error = %v", err)
	}
}
