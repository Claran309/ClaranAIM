//go:build integration

package service

import (
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/outbox"
	"context"
	"strings"
	"testing"
)

func TestIntegrationEditMessageWritesEditRecordAndOutbox(t *testing.T) {
	svc, db := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "before edit",
		ClientMsgID:    "it-edit-1",
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	edited, err := svc.EditMessage(context.Background(), msg.ID, 101, "after edit")
	if err != nil {
		t.Fatalf("EditMessage() error = %v", err)
	}
	if edited.Content != "after edit" || !edited.IsEdited {
		t.Fatalf("edited message = %#v", edited)
	}
	var record model.MessageEditRecord
	if err := db.Where("message_id = ?", msg.ID).First(&record).Error; err != nil {
		t.Fatalf("edit record missing: %v", err)
	}
	if record.OldContent != "before edit" || record.NewContent != "after edit" || record.EditorID != 101 {
		t.Fatalf("edit record = %#v", record)
	}
	var event outbox.Event
	if err := db.Where("aggregate_id = ? AND event_type = ?", msg.ID, events.EventTypeMessageEdited).First(&event).Error; err != nil {
		t.Fatalf("message.edited outbox event missing: %v", err)
	}
}

func TestIntegrationEditMessageRejectsNonSender(t *testing.T) {
	svc, _ := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "owned by 101",
		ClientMsgID:    "it-edit-2",
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	if _, err := svc.EditMessage(context.Background(), msg.ID, 102, "hijack"); err == nil {
		t.Fatal("EditMessage() by non-sender succeeded, want error")
	}
}

func TestIntegrationRecallMessageKeepsFactAndWritesOutbox(t *testing.T) {
	svc, db := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "to recall",
		ClientMsgID:    "it-recall-1",
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	if err := svc.RecallMessage(context.Background(), msg.ID, 101); err != nil {
		t.Fatalf("RecallMessage() error = %v", err)
	}
	var stored model.Message
	if err := db.First(&stored, msg.ID).Error; err != nil {
		t.Fatalf("message row should be kept for audit: %v", err)
	}
	if stored.Status != MessageStatusRecalled {
		t.Fatalf("status = %q, want recalled", stored.Status)
	}
	var event outbox.Event
	if err := db.Where("aggregate_id = ? AND event_type = ?", msg.ID, events.EventTypeMessageRecalled).First(&event).Error; err != nil {
		t.Fatalf("message.recalled outbox event missing: %v", err)
	}
}

func TestIntegrationRecallMessageRejectsOtherUser(t *testing.T) {
	svc, _ := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "cannot recall by other",
		ClientMsgID:    "it-recall-2",
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	if err := svc.RecallMessage(context.Background(), msg.ID, 102); err == nil {
		t.Fatal("RecallMessage() by other user succeeded, want error")
	}
}

func TestIntegrationDeleteLocalMessageOnlyAffectsOneUser(t *testing.T) {
	svc, db := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "local delete",
		ClientMsgID:    "it-local-delete-1",
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	if err := svc.DeleteLocalMessage(context.Background(), conv.ID, 102, msg.ID); err != nil {
		t.Fatalf("DeleteLocalMessage() error = %v", err)
	}
	var deletedState model.MessageUserState
	if err := db.Where("message_id = ? AND user_id = ?", msg.ID, 102).First(&deletedState).Error; err != nil {
		t.Fatalf("user 102 state missing: %v", err)
	}
	if deletedState.LocalDeletedAt == nil {
		t.Fatalf("user 102 state = %#v, want deleted", deletedState)
	}
	var senderState model.MessageUserState
	if err := db.Where("message_id = ? AND user_id = ?", msg.ID, 101).First(&senderState).Error; err != nil {
		t.Fatalf("sender state missing: %v", err)
	}
	if senderState.LocalDeletedAt != nil {
		t.Fatal("sender state must not be marked deleted")
	}
}

func TestIntegrationSearchMessagesFindsContentAndExcludesRecalled(t *testing.T) {
	svc, _ := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	keep, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "unique-search-token keep",
		ClientMsgID:    "it-search-1",
	})
	if err != nil {
		t.Fatalf("SendMessageExt(keep) error = %v", err)
	}
	recalled, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "unique-search-token recalled",
		ClientMsgID:    "it-search-2",
	})
	if err != nil {
		t.Fatalf("SendMessageExt(recalled) error = %v", err)
	}
	if err := svc.RecallMessage(context.Background(), recalled.ID, 101); err != nil {
		t.Fatalf("RecallMessage() error = %v", err)
	}
	found, err := svc.SearchMessages(context.Background(), 102, "unique-search-token", 20)
	if err != nil {
		t.Fatalf("SearchMessages() error = %v", err)
	}
	var keepFound, recalledFound bool
	for _, item := range found {
		if item.ID == keep.ID {
			keepFound = true
		}
		if item.ID == recalled.ID {
			recalledFound = true
		}
	}
	if !keepFound {
		t.Fatalf("search did not return visible message: %#v", found)
	}
	if recalledFound {
		t.Fatal("search returned recalled message")
	}
}

func TestIntegrationGetHistoryRespectsParticipantBoundary(t *testing.T) {
	svc, _ := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	if _, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "history message",
		ClientMsgID:    "it-history-1",
	}); err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	history, err := svc.GetHistory(context.Background(), conv.ID, 102, 20, 0)
	if err != nil {
		t.Fatalf("GetHistory() error = %v", err)
	}
	if len(history) != 1 || !strings.Contains(history[0].Content, "history message") {
		t.Fatalf("history = %#v", history)
	}
	if _, err := svc.GetHistory(context.Background(), conv.ID, 999, 20, 0); err == nil {
		t.Fatal("GetHistory() for non-participant succeeded, want error")
	}
}

// TestIntegrationReadCursorIsMonotonicUnderOutOfOrderAcks 覆盖 A1 的“乱序补偿”：
// 移动端晚到的旧 ACK 不能把已读游标推回更早的消息。
func TestIntegrationReadCursorIsMonotonicUnderOutOfOrderAcks(t *testing.T) {
	svc, db := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	first, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "ack order 1",
		ClientMsgID:    "it-order-1",
	})
	if err != nil {
		t.Fatalf("SendMessageExt(first) error = %v", err)
	}
	second, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "ack order 2",
		ClientMsgID:    "it-order-2",
	})
	if err != nil {
		t.Fatalf("SendMessageExt(second) error = %v", err)
	}
	if err := svc.MarkConversationRead(context.Background(), conv.ID, 102, second.ID); err != nil {
		t.Fatalf("MarkConversationRead(newest) error = %v", err)
	}
	if err := svc.MarkConversationRead(context.Background(), conv.ID, 102, first.ID); err != nil {
		t.Fatalf("MarkConversationRead(stale ack) error = %v", err)
	}
	var participant model.ConversationParticipant
	if err := db.Where("conversation_id = ? AND user_id = ?", conv.ID, 102).First(&participant).Error; err != nil {
		t.Fatalf("load participant error = %v", err)
	}
	if participant.LastReadMessageID != second.ID {
		t.Fatalf("cursor regressed to %d, want %d (stale ack must not rewind)", participant.LastReadMessageID, second.ID)
	}
	var staleState model.MessageUserState
	if err := db.Where("message_id = ? AND user_id = ?", first.ID, 102).First(&staleState).Error; err != nil {
		t.Fatalf("load state error = %v", err)
	}
	if staleState.ReadAt == nil {
		t.Fatal("earlier message should still be marked read by the newer ack")
	}
}

// TestIntegrationDuplicateAckFromSecondDeviceDoesNotDuplicateReceipt 覆盖 A1 的“多端去重”：
// 同一用户的两台设备重复上报同一条已读时，只应产生一条 message.read 事件。
func TestIntegrationDuplicateAckFromSecondDeviceDoesNotDuplicateReceipt(t *testing.T) {
	svc, db := setupIntegrationService(t)
	conv := createIntegrationConversation(t, svc)
	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       101,
		Content:        "multi device ack",
		ClientMsgID:    "it-multi-1",
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := svc.MarkConversationRead(context.Background(), conv.ID, 102, msg.ID); err != nil {
			t.Fatalf("MarkConversationRead(device %d) error = %v", i+1, err)
		}
	}
	var readEvents int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_id = ? AND event_type = ?", msg.ID, events.EventTypeMessageRead).
		Count(&readEvents).Error; err != nil {
		t.Fatalf("count read events error = %v", err)
	}
	if readEvents != 1 {
		t.Fatalf("message.read events = %d, want 1 (duplicate acks must be deduped)", readEvents)
	}
}
