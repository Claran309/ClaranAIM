package eventconsumer

import (
	"ClaranAIM/internal/bot-manager-service/model"
	"ClaranAIM/internal/bot-manager-service/service"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"
)

func TestHandleAgentMentionEventHandlesNilSendMessageResponse(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID: 10,
		SenderID:       1001,
		Content:        "@agent help",
		MsgID:          99,
		MentionUserIDs: []int64{2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	dispatchRepo := &fakeDispatchRepo{}

	err = handleAgentMentionEvent(context.Background(), envelope, &fakeBotService{
		bot:   &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true},
		reply: "reply",
	}, dispatchRepo, &fakeMessageClient{sendResp: nil})
	if err == nil || !strings.Contains(err.Error(), "空响应") {
		t.Fatalf("handleAgentMentionEvent error = %v, want nil response error", err)
	}
	if dispatchRepo.failed == "" {
		t.Fatal("expected failed dispatch record")
	}
}

func TestHandleAgentMentionEventSkipsPermanentPermissionError(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID: 10,
		SenderID:       1001,
		Content:        "@agent help",
		MsgID:          99,
		MentionUserIDs: []int64{2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	dispatchRepo := &fakeDispatchRepo{}

	err = handleAgentMentionEvent(context.Background(), envelope, &fakeBotService{
		bot:     &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true},
		chatErr: errors.New("无权操作该Agent"),
	}, dispatchRepo, &fakeMessageClient{})
	if err != nil {
		t.Fatalf("handleAgentMentionEvent returned error for permanent denial: %v", err)
	}
	if dispatchRepo.failed == "" {
		t.Fatal("expected failed dispatch record for audit")
	}
}

func TestHandleAgentMentionEventTriggersPrivateAgentConversation(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "private",
		SenderID:         1001,
		Content:          "帮我总结一下刚才的话",
		MsgID:            99,
		ParticipantIDs:   []int64{1001, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{
		bot:   &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true},
		reply: "reply",
	}
	msgClient := &fakeMessageClient{
		sendResp: &message.SendMessageResp{Success: true, MsgId: 101},
		historyResp: &message.GetHistoryResp{Success: true, Messages: []*message.Message{
			{SenderId: 1001, Content: "第一句背景", CreatedAt: "2026-05-27 10:00:00"},
			{SenderId: 2001, Content: "上一轮回复", CreatedAt: "2026-05-27 10:00:10"},
		}},
	}

	err = handleAgentMentionEvent(context.Background(), envelope, botSvc, &fakeDispatchRepo{}, msgClient)
	if err != nil {
		t.Fatalf("handleAgentMentionEvent returned error: %v", err)
	}
	if botSvc.chatCalls != 1 {
		t.Fatalf("chat calls = %d, want 1", botSvc.chatCalls)
	}
	if !strings.Contains(botSvc.lastMessage, "会话材料") || !strings.Contains(botSvc.lastMessage, "第一句背景") {
		t.Fatalf("agent input did not include conversation context: %q", botSvc.lastMessage)
	}
	if msgClient.lastSend == nil || msgClient.lastSend.SenderId != 2001 {
		t.Fatalf("agent reply send request = %#v, want sender 2001", msgClient.lastSend)
	}
}

func TestHandleAgentMentionEventInjectsGroupContext(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         1001,
		Content:          "@agent 你怎么看？",
		MsgID:            99,
		MentionUserIDs:   []int64{2001},
		ParticipantIDs:   []int64{1001, 1002, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{
		bot:   &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true},
		reply: "reply",
	}
	msgClient := &fakeMessageClient{
		sendResp: &message.SendMessageResp{Success: true, MsgId: 101},
		historyResp: &message.GetHistoryResp{Success: true, Messages: []*message.Message{
			{SenderId: 1002, Content: "群聊里的关键背景", CreatedAt: "2026-05-27 10:00:00"},
			{SenderId: 1001, Content: "@agent 你怎么看？", CreatedAt: "2026-05-27 10:00:10"},
		}},
	}

	err = handleAgentMentionEvent(context.Background(), envelope, botSvc, &fakeDispatchRepo{}, msgClient)
	if err != nil {
		t.Fatalf("handleAgentMentionEvent returned error: %v", err)
	}
	if !strings.Contains(botSvc.lastMessage, "群聊里的关键背景") {
		t.Fatalf("agent input did not include group context: %q", botSvc.lastMessage)
	}
}

type fakeDispatchRepo struct {
	failed string
}

func (r *fakeDispatchRepo) Start(ctx context.Context, record *model.AgentDispatchRecord) (bool, error) {
	return true, nil
}

func (r *fakeDispatchRepo) MarkCompleted(ctx context.Context, eventID string, agentUserID, replyMsgID int64) error {
	return nil
}

func (r *fakeDispatchRepo) MarkFailed(ctx context.Context, eventID string, agentUserID int64, message string) error {
	r.failed = message
	return nil
}

type fakeBotService struct {
	bot         *model.Bot
	reply       string
	chatErr     error
	chatCalls   int
	lastMessage string
}

func (s *fakeBotService) CreateBot(context.Context, string, string, string, string, string, string, string, string, string, string, string, string, string, int64, string, string, string) (*model.Bot, error) {
	return nil, nil
}
func (s *fakeBotService) UpdateBot(context.Context, int64, int64, string, string, string, string, string, string, string, string, string, string, string, string, bool, bool, string, string, string) error {
	return nil
}
func (s *fakeBotService) GetBot(context.Context, int64) (*model.Bot, error) { return nil, nil }
func (s *fakeBotService) ListBots(context.Context, int64, string) ([]model.Bot, error) {
	return nil, nil
}
func (s *fakeBotService) DeleteBot(context.Context, int64, int64) error { return nil }
func (s *fakeBotService) ChatWithBot(_ context.Context, _ int64, _ int64, _ int64, message string) (*service.ChatResult, error) {
	s.chatCalls++
	s.lastMessage = message
	if s.chatErr != nil {
		return nil, s.chatErr
	}
	return &service.ChatResult{Reply: s.reply, ConversationID: 10}, nil
}
func (s *fakeBotService) CreateRoute(context.Context, int64, string, string, int64) (*model.BotRoute, error) {
	return nil, nil
}
func (s *fakeBotService) ListRoutes(context.Context, int64) ([]model.BotRoute, error) {
	return nil, nil
}
func (s *fakeBotService) DeleteRoute(context.Context, int64, int64) error { return nil }
func (s *fakeBotService) GetBilling(context.Context, int64, int64, int64, int64) ([]model.BillingRecord, int64, error) {
	return nil, 0, nil
}
func (s *fakeBotService) GrantPermission(context.Context, int64, int64, int64, string) error {
	return nil
}
func (s *fakeBotService) RevokePermission(context.Context, int64, int64, int64) error {
	return nil
}
func (s *fakeBotService) ListPermissions(context.Context, int64, int64) ([]model.BotPermission, error) {
	return nil, nil
}
func (s *fakeBotService) RunAgentTask(context.Context, int64, int64, int64, string, string) (string, error) {
	return "", nil
}
func (s *fakeBotService) GetBotByAgentUserID(ctx context.Context, agentUserID int64) (*model.Bot, error) {
	if s.bot != nil && s.bot.AgentUserID == agentUserID {
		return s.bot, nil
	}
	return nil, nil
}

type fakeMessageClient struct {
	sendResp    *message.SendMessageResp
	historyResp *message.GetHistoryResp
	lastSend    *message.SendMessageReq
}

func (c *fakeMessageClient) CreateConversation(ctx context.Context, req *message.CreateConversationReq, callOptions ...callopt.Option) (*message.CreateConversationResp, error) {
	return nil, nil
}
func (c *fakeMessageClient) GetConversation(ctx context.Context, req *message.GetConversationReq, callOptions ...callopt.Option) (*message.GetConversationResp, error) {
	return nil, nil
}
func (c *fakeMessageClient) GetUserConversations(ctx context.Context, req *message.GetUserConversationsReq, callOptions ...callopt.Option) (*message.GetUserConversationsResp, error) {
	return nil, nil
}
func (c *fakeMessageClient) SendMessage(ctx context.Context, req *message.SendMessageReq, callOptions ...callopt.Option) (*message.SendMessageResp, error) {
	c.lastSend = req
	return c.sendResp, nil
}
func (c *fakeMessageClient) MarkConversationRead(ctx context.Context, req *message.MarkConversationReadReq, callOptions ...callopt.Option) (*message.MarkConversationReadResp, error) {
	return nil, nil
}
func (c *fakeMessageClient) DeleteLocalMessage(ctx context.Context, req *message.DeleteLocalMessageReq, callOptions ...callopt.Option) (*message.DeleteLocalMessageResp, error) {
	return nil, nil
}
func (c *fakeMessageClient) EditMessage(ctx context.Context, req *message.EditMessageReq, callOptions ...callopt.Option) (*message.EditMessageResp, error) {
	return nil, nil
}
func (c *fakeMessageClient) RecallMessage(ctx context.Context, req *message.RecallMessageReq, callOptions ...callopt.Option) (*message.RecallMessageResp, error) {
	return nil, nil
}
func (c *fakeMessageClient) GetHistory(ctx context.Context, req *message.GetHistoryReq, callOptions ...callopt.Option) (*message.GetHistoryResp, error) {
	return c.historyResp, nil
}
func (c *fakeMessageClient) SearchMessages(ctx context.Context, req *message.SearchMessagesReq, callOptions ...callopt.Option) (*message.SearchMessagesResp, error) {
	return nil, nil
}
func (c *fakeMessageClient) GetConversationParticipants(ctx context.Context, req *message.GetConversationParticipantsReq, callOptions ...callopt.Option) (*message.GetConversationParticipantsResp, error) {
	return nil, nil
}
