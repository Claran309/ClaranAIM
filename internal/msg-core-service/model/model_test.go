package model

import (
	"reflect"
	"testing"
	"time"
)

func TestOptionalMessageTimesAreNullable(t *testing.T) {
	messageType := reflect.TypeOf(Message{})
	editedAt, ok := messageType.FieldByName("EditedAt")
	if !ok {
		t.Fatal("Message.EditedAt field is missing")
	}
	if editedAt.Type != reflect.TypeOf((*time.Time)(nil)) {
		t.Fatalf("Message.EditedAt type = %s, want *time.Time so unedited messages write NULL instead of zero datetime", editedAt.Type)
	}

	participantType := reflect.TypeOf(ConversationParticipant{})
	lastReadAt, ok := participantType.FieldByName("LastReadAt")
	if !ok {
		t.Fatal("ConversationParticipant.LastReadAt field is missing")
	}
	if lastReadAt.Type != reflect.TypeOf((*time.Time)(nil)) {
		t.Fatalf("ConversationParticipant.LastReadAt type = %s, want *time.Time so unread conversations write NULL instead of zero datetime", lastReadAt.Type)
	}
}
