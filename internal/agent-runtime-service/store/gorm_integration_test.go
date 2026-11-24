//go:build integration

package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

const integrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupIntegrationStore(t *testing.T) *GormStore {
	t.Helper()
	dsn := os.Getenv(integrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 runtime store MySQL 集成测试", integrationDSNEnv)
	}
	db, err := InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	for _, table := range []string{"agent_messages", "agent_sessions", "agent_runs", "agent_checkpoints"} {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clean table %s error = %v", table, err)
		}
	}
	return NewGormStore(db)
}

func TestIntegrationSessionRollingSummaryAndCrossInstanceRead(t *testing.T) {
	first := setupIntegrationStore(t)
	ctx := context.Background()
	sessionID := fmt.Sprintf("it-session-%d", time.Now().UnixNano())
	owner := SessionOwner{BotID: 71, UserID: 1001, ConversationID: 2001}

	for i := 0; i < 30; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		content := fmt.Sprintf("%s message %02d with enough content for summary", role, i)
		if err := first.AppendMessage(ctx, sessionID, owner, role, content, int64(i)); err != nil {
			t.Fatalf("AppendMessage(%d) error = %v", i, err)
		}
	}

	summary, messages, err := first.Context(ctx, sessionID, DefaultContextLimit)
	if err != nil {
		t.Fatalf("Context() error = %v", err)
	}
	if summary == "" {
		t.Fatal("rolling summary is empty after 30 messages")
	}
	if len(messages) == 0 || len(messages) > DefaultContextLimit {
		t.Fatalf("context messages = %d, want 1..%d", len(messages), DefaultContextLimit)
	}

	second := NewGormStore(first.db)
	summary2, messages2, err := second.Context(ctx, sessionID, DefaultContextLimit)
	if err != nil {
		t.Fatalf("second Context() error = %v", err)
	}
	if summary2 != summary {
		t.Fatalf("two instances returned different summaries")
	}
	if len(messages2) != len(messages) || messages2[len(messages2)-1].Content != messages[len(messages)-1].Content {
		t.Fatalf("two instances returned different recent messages")
	}

	session, err := second.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if session == nil || len(session.Messages) != 30 {
		t.Fatalf("persisted messages = %d, want 30", len(session.Messages))
	}
	if session.Title == "" || session.Title == "新会话" {
		t.Fatalf("session title = %q, want derived title", session.Title)
	}
}

func TestIntegrationSessionOwnerIsolation(t *testing.T) {
	store := setupIntegrationStore(t)
	ctx := context.Background()
	sessionID := fmt.Sprintf("it-owner-%d", time.Now().UnixNano())
	if err := store.AppendMessage(ctx, sessionID, SessionOwner{BotID: 71, UserID: 1001}, "user", "hello", 1); err != nil {
		t.Fatalf("AppendMessage() error = %v", err)
	}
	err := store.AppendMessage(ctx, sessionID, SessionOwner{BotID: 72, UserID: 1001}, "user", "collision", 1)
	if err == nil {
		t.Fatal("AppendMessage() with different bot succeeded, want owner isolation error")
	}
}

func TestIntegrationRunAndCheckpointPersistence(t *testing.T) {
	first := setupIntegrationStore(t)
	ctx := context.Background()
	runID := fmt.Sprintf("it-run-%d", time.Now().UnixNano())
	run := &RunInfo{
		RunID:          runID,
		SessionID:      "it-session-run",
		BotID:          71,
		UserID:         1001,
		ConversationID: 2001,
		Status:         RunStatusRunning,
		StartedAt:      time.Now(),
	}
	if err := first.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	now := time.Now()
	run.Status = RunStatusCompleted
	run.Step = 3
	run.InputTokens = 120
	run.OutputTokens = 45
	run.CostUSD = 0.012
	run.FinishedAt = &now
	if err := first.UpdateRun(ctx, run); err != nil {
		t.Fatalf("UpdateRun() error = %v", err)
	}

	second := NewGormStore(first.db)
	got, err := second.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got == nil || got.Status != RunStatusCompleted || got.Step != 3 || got.InputTokens != 120 || got.OutputTokens != 45 {
		t.Fatalf("run from second instance = %#v", got)
	}

	checkpointID := fmt.Sprintf("it-cp-%d", time.Now().UnixNano())
	payload := []byte("checkpoint-payload")
	if err := first.Set(ctx, checkpointID, payload); err != nil {
		t.Fatalf("Set checkpoint error = %v", err)
	}
	loaded, ok, err := second.Get(ctx, checkpointID)
	if err != nil || !ok || string(loaded) != string(payload) {
		t.Fatalf("checkpoint from second instance = %q/%v/%v", loaded, ok, err)
	}
}
