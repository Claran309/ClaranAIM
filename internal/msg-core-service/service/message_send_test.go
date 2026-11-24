package service

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/pkg/outbox"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMessageRepo 只实现 SendMessage 链路用到的仓储方法。
// 其余方法通过嵌入 dao.MessageRepository 接口保留编译期契约，避免在单元测试里伪造整个存储层。
type fakeMessageRepo struct {
	dao.MessageRepository

	mu             sync.Mutex
	conversation   *model.Conversation
	participants   []model.ConversationParticipant
	messages       []*model.Message
	messagesByKey  map[string]*model.Message
	userStates     []*model.MessageUserState
	outboxEvents   []outbox.Event
	txDepth        int
	outsideTxCalls []string
	failCreate     error
	failOutbox     error
}

func newFakeMessageRepo() *fakeMessageRepo {
	return &fakeMessageRepo{
		conversation:  &model.Conversation{ID: 10, Type: "private"},
		participants:  []model.ConversationParticipant{{ConversationID: 10, UserID: 1}, {ConversationID: 10, UserID: 2}},
		messagesByKey: make(map[string]*model.Message),
	}
}

func (f *fakeMessageRepo) ensureTx(method string) error {
	if f.txDepth == 0 {
		f.outsideTxCalls = append(f.outsideTxCalls, method)
	}
	return nil
}

func (f *fakeMessageRepo) GetMessageByClientMsgID(_ context.Context, clientMsgID string) (*model.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.messagesByKey[clientMsgID], nil
}

func (f *fakeMessageRepo) GetConversationByID(_ context.Context, id int64) (*model.Conversation, error) {
	if f.conversation == nil || f.conversation.ID != id {
		return nil, nil
	}
	return f.conversation, nil
}

func (f *fakeMessageRepo) GetParticipants(_ context.Context, _ int64) ([]model.ConversationParticipant, error) {
	return f.participants, nil
}

func (f *fakeMessageRepo) WithTransaction(_ context.Context, fn func(tx dao.MessageRepository) error) error {
	f.mu.Lock()
	f.txDepth++
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.txDepth--
		f.mu.Unlock()
	}()
	return fn(f)
}

func (f *fakeMessageRepo) CreateMessage(_ context.Context, msg *model.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.ensureTx("CreateMessage"); err != nil {
		return err
	}
	if f.failCreate != nil {
		return f.failCreate
	}
	if msg.ID == 0 {
		msg.ID = 1001
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.ClientMsgID != nil {
		f.messagesByKey[*msg.ClientMsgID] = msg
	}
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeMessageRepo) UpdateConversation(_ context.Context, _ *model.Conversation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ensureTx("UpdateConversation")
}

func (f *fakeMessageRepo) UpsertMessageUserState(_ context.Context, state *model.MessageUserState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.ensureTx("UpsertMessageUserState"); err != nil {
		return err
	}
	f.userStates = append(f.userStates, state)
	return nil
}

func (f *fakeMessageRepo) SaveOutboxEvent(_ context.Context, event outbox.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.ensureTx("SaveOutboxEvent"); err != nil {
		return err
	}
	if f.failOutbox != nil {
		return f.failOutbox
	}
	f.outboxEvents = append(f.outboxEvents, event)
	return nil
}

func TestSendMessageWritesMessageAndOutboxInSameTransaction(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := NewMessageServiceForTest(repo)

	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: 10,
		SenderID:       1,
		Content:        "hello",
		MsgType:        "text",
		ClientMsgID:    "client-1",
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	if msg.ID == 0 || msg.Content != "hello" {
		t.Fatalf("unexpected message: %#v", msg)
	}
	if len(repo.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(repo.messages))
	}
	if len(repo.outboxEvents) != 1 {
		t.Fatalf("outbox events = %d, want 1", len(repo.outboxEvents))
	}
	if got := repo.outboxEvents[0].EventType; got == "" {
		t.Fatal("outbox event type is empty")
	}
	if len(repo.outsideTxCalls) != 0 {
		t.Fatalf("writes outside transaction: %v", repo.outsideTxCalls)
	}
	if len(repo.userStates) != len(repo.participants) {
		t.Fatalf("message user states = %d, want %d", len(repo.userStates), len(repo.participants))
	}
}

func TestSendMessageIsIdempotentByClientMsgID(t *testing.T) {
	repo := newFakeMessageRepo()
	existing := &model.Message{ID: 9001, ConversationID: 10, SenderID: 1, Content: "first", MsgType: "text"}
	clientMsgID := "client-duplicate"
	existing.ClientMsgID = &clientMsgID
	repo.messagesByKey[clientMsgID] = existing
	svc := NewMessageServiceForTest(repo)

	got, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: 10,
		SenderID:       1,
		Content:        "second",
		ClientMsgID:    clientMsgID,
	})
	if err != nil {
		t.Fatalf("SendMessageExt() error = %v", err)
	}
	if got.ID != existing.ID || got.Content != "first" {
		t.Fatalf("duplicate send returned %#v, want existing message", got)
	}
	if len(repo.messages) != 0 || len(repo.outboxEvents) != 0 {
		t.Fatal("duplicate send must not create another message or outbox event")
	}
}

func TestSendMessageRejectsClientMsgIDOwnedByAnotherSender(t *testing.T) {
	repo := newFakeMessageRepo()
	clientMsgID := "client-owned"
	repo.messagesByKey[clientMsgID] = &model.Message{ID: 9002, ConversationID: 10, SenderID: 2}
	svc := NewMessageServiceForTest(repo)

	_, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: 10,
		SenderID:       1,
		Content:        "reuse",
		ClientMsgID:    clientMsgID,
	})
	if err == nil || !strings.Contains(err.Error(), "client_msg_id") {
		t.Fatalf("SendMessageExt() error = %v, want client_msg_id conflict", err)
	}
}

func TestSendMessagePropagatesOutboxFailure(t *testing.T) {
	repo := newFakeMessageRepo()
	repo.failOutbox = errors.New("outbox insert failed")
	svc := NewMessageServiceForTest(repo)

	_, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: 10,
		SenderID:       1,
		Content:        "will fail",
	})
	if err == nil || !strings.Contains(err.Error(), "outbox insert failed") {
		t.Fatalf("SendMessageExt() error = %v, want outbox failure", err)
	}
}

func TestSendMessageRejectsNonParticipant(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := NewMessageServiceForTest(repo)

	_, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: 10,
		SenderID:       99,
		Content:        "intruder",
	})
	if !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("SendMessageExt() error = %v, want errConversationAccessDenied", err)
	}
	if len(repo.messages) != 0 {
		t.Fatal("rejected send must not persist a message")
	}
}
