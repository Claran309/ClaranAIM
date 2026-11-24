//go:build integration

package service

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/outbox"
	"context"
	"errors"
	"os"
	"testing"

	"gorm.io/gorm"
)

const integrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupIntegrationService(t *testing.T) (*messageServiceImpl, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv(integrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 MySQL 集成测试", integrationDSNEnv)
	}
	db, err := dao.InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	resetMessageTables(t, db)
	return NewMessageServiceForTest(dao.NewMessageRepo(db)), db
}

func resetMessageTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	tables := []string{
		"event_outbox",
		"message_user_states",
		"message_edit_records",
		"message_translations",
		"messages",
		"conversation_participants",
		"conversations",
	}
	for _, table := range tables {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clean table %s error = %v", table, err)
		}
	}
}

func createIntegrationConversation(t *testing.T, svc *messageServiceImpl) *model.Conversation {
	t.Helper()
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{101, 102}, 0)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	return conv
}

func TestIntegrationSendMessageAndOutboxCommitTogether(t *testing.T) {
	svc, db := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)

	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "integration hello",
		MsgType:        "text",
		ClientMsgID:    "integration-commit-1",
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}

	var stored model.Message
	if err := db.First(&stored, msg.ID).Error; err != nil {
		t.Fatalf("message not persisted: %v", err)
	}
	if stored.Content != "integration hello" {
		t.Fatalf("stored content = %q", stored.Content)
	}

	var event outbox.Event
	if err := db.Where("aggregate_id = ? AND event_type = ?", msg.ID, events.EventTypeMessageCreated).First(&event).Error; err != nil {
		t.Fatalf("outbox event not persisted: %v", err)
	}
	if event.Status != outbox.StatusPending {
		t.Fatalf("outbox status = %q, want pending", event.Status)
	}
	if event.AggregateType != "message" {
		t.Fatalf("aggregate type = %q, want message", event.AggregateType)
	}
}

func TestIntegrationSendMessageIsIdempotent(t *testing.T) {
	svc, db := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)

	first, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "first",
		ClientMsgID:    "integration-idempotent-1",
	})
	if err != nil {
		t.Fatalf("first SendMessageExt() error = %v", err)
	}
	second, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "second",
		ClientMsgID:    "integration-idempotent-1",
	})
	if err != nil {
		t.Fatalf("second SendMessageExt() error = %v", err)
	}
	if first.ID != second.ID || second.Content != "first" {
		t.Fatalf("duplicate send returned %#v, want existing message %#v", second, first)
	}

	var messageCount int64
	if err := db.Model(&model.Message{}).Where("client_msg_id = ?", "integration-idempotent-1").Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages error = %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("message count = %d, want 1", messageCount)
	}
	var outboxCount int64
	if err := db.Model(&outbox.Event{}).Where("aggregate_id = ?", first.ID).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count outbox error = %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("outbox count = %d, want 1", outboxCount)
	}
}

type failingOutboxRepo struct {
	dao.MessageRepository
}

func (r *failingOutboxRepo) WithTransaction(ctx context.Context, fn func(tx dao.MessageRepository) error) error {
	return r.MessageRepository.WithTransaction(ctx, func(tx dao.MessageRepository) error {
		return fn(&failingOutboxRepo{MessageRepository: tx})
	})
}

func (r *failingOutboxRepo) SaveOutboxEvent(context.Context, outbox.Event) error {
	return errors.New("forced outbox failure")
}

func TestIntegrationSendMessageRollsBackWhenOutboxFails(t *testing.T) {
	_, db := setupIntegrationService(t)
	svc := NewMessageServiceForTest(&failingOutboxRepo{MessageRepository: dao.NewMessageRepo(db)})
	conv := createIntegrationConversation(t, svc)

	_, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "must rollback",
		ClientMsgID:    "integration-rollback-1",
	})
	if err == nil {
		t.Fatal("SendMessageExt() succeeded, want outbox failure")
	}

	var messageCount int64
	if err := db.Model(&model.Message{}).Where("client_msg_id = ?", "integration-rollback-1").Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages error = %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("message count = %d, want 0 after rollback", messageCount)
	}
	var outboxCount int64
	if err := db.Model(&outbox.Event{}).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count outbox error = %v", err)
	}
	if outboxCount != 0 {
		t.Fatalf("outbox count = %d, want 0 after rollback", outboxCount)
	}
}

func TestIntegrationMarkConversationReadAdvancesUserCursor(t *testing.T) {
	svc, db := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)

	first, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "first",
		ClientMsgID:    "integration-read-1",
	})
	if err != nil {
		t.Fatalf("send first message error = %v", err)
	}
	second, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "second",
		ClientMsgID:    "integration-read-2",
	})
	if err != nil {
		t.Fatalf("send second message error = %v", err)
	}

	if err := svc.MarkConversationRead(context.Background(), conv.ID, 102, second.ID); err != nil {
		t.Fatalf("MarkConversationRead() error = %v", err)
	}

	var participant model.ConversationParticipant
	if err := db.Where("conversation_id = ? AND user_id = ?", conv.ID, 102).First(&participant).Error; err != nil {
		t.Fatalf("load participant error = %v", err)
	}
	if participant.LastReadMessageID != second.ID || participant.LastReadAt == nil {
		t.Fatalf("participant cursor = (%d, %v), want (%d, non-nil)", participant.LastReadMessageID, participant.LastReadAt, second.ID)
	}

	for _, messageID := range []int64{first.ID, second.ID} {
		var state model.MessageUserState
		if err := db.Where("message_id = ? AND user_id = ?", messageID, 102).First(&state).Error; err != nil {
			t.Fatalf("load message state %d error = %v", messageID, err)
		}
		if state.ReadAt == nil {
			t.Fatalf("message %d read_at is nil, want non-nil", messageID)
		}
	}
}
