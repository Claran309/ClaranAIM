package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"testing"
)

func TestMemoryPublisherStoresPublishedEnvelopes(t *testing.T) {
	publisher := NewMemoryPublisher()
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		Type:           "new_message",
		ConversationID: 10,
		MsgID:          20,
		TargetUserIDs:  []int64{1, 2},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}

	if err := publisher.Publish(context.Background(), envelope); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	published := publisher.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	if published[0].Type != events.EventTypeMessageCreated {
		t.Fatalf("published type = %q, want %q", published[0].Type, events.EventTypeMessageCreated)
	}
}
