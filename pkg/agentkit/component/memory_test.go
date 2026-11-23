package component

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestStorePersistsSessionAcrossReload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	session, err := store.GetSession("conv-1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got := session.GetTitle(); got != "新会话" {
		t.Fatalf("GetTitle() = %q, want 新会话", got)
	}

	messages := []*schema.Message{
		{Role: schema.User, Content: "你好，介绍一下 ClaranAIM"},
		{Role: schema.Assistant, Content: "ClaranAIM 是一个 Agent-Native AIM 系统"},
	}
	for _, msg := range messages {
		if err := session.Append(msg); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore(reload) error = %v", err)
	}
	reloaded, err := reloadedStore.GetSession("conv-1")
	if err != nil {
		t.Fatalf("GetSession(reload) error = %v", err)
	}

	got := reloaded.GetMessages()
	if len(got) != len(messages) {
		t.Fatalf("GetMessages() length = %d, want %d", len(got), len(messages))
	}
	for i, msg := range messages {
		if got[i].Role != msg.Role || got[i].Content != msg.Content {
			t.Fatalf("message[%d] = (%s, %q), want (%s, %q)", i, got[i].Role, got[i].Content, msg.Role, msg.Content)
		}
	}
	if got := reloaded.GetTitle(); got != messages[0].Content {
		t.Fatalf("GetTitle() = %q, want %q", got, messages[0].Content)
	}

	metas, err := reloadedStore.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(metas) != 1 || metas[0].ID != "conv-1" {
		t.Fatalf("ListSessions() = %#v, want one session conv-1", metas)
	}
}

func TestSessionTitleTruncatesLongUserMessage(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	session, err := store.GetSession("conv-long")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}

	long := make([]rune, 80)
	for i := range long {
		long[i] = '鱼'
	}
	if err := session.Append(&schema.Message{Role: schema.User, Content: string(long)}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	want := string(long[:60]) + "..."
	if got := session.GetTitle(); got != want {
		t.Fatalf("GetTitle() length = %d, want truncated title length %d", len([]rune(got)), len([]rune(want)))
	}
}

func TestStoreDeleteSessionRemovesFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, err := store.GetSession("conv-delete"); err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}

	path := filepath.Join(dir, "conv-delete.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session file missing before delete: %v", err)
	}
	if err := store.DeleteSession("conv-delete"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("session file still exists after delete: %v", err)
	}
}

func TestSessionAppendIsConcurrencySafe(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	session, err := store.GetSession("conv-concurrent")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}

	const writers = 50
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			if err := session.Append(&schema.Message{Role: schema.User, Content: "并发消息"}); err != nil {
				t.Errorf("Append() error = %v", err)
			}
		}()
	}
	wg.Wait()

	if got := len(session.GetMessages()); got != writers {
		t.Fatalf("GetMessages() length = %d, want %d", got, writers)
	}
}
