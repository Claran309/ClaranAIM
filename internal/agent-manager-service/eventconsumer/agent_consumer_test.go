package eventconsumer

import (
	"ClaranAIM/internal/agent-manager-service/model"
	"ClaranAIM/internal/agent-manager-service/service"
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

func TestHandleAgentMentionEventSkipsPermanentProviderAuthError(t *testing.T) {
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
		chatErr: errors.New("Agent执行失败: [NodeRunError] error, status code: 401, status: 401 Unauthorized, message: 身份验证失败。"),
	}, dispatchRepo, &fakeMessageClient{})
	if err != nil {
		t.Fatalf("handleAgentMentionEvent returned error for provider auth failure: %v", err)
	}
	if dispatchRepo.failed == "" {
		t.Fatal("expected failed dispatch record for provider auth failure")
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

func TestHandleAgentMentionEventSkipsPrivateAgentSelfMessageWithoutClientMsgID(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "private",
		SenderID:         2001,
		Content:          "这是 Agent 自己刚发出的回复",
		MsgID:            100,
		ParticipantIDs:   []int64{1001, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}, reply: "reply"}

	err = handleAgentMentionEvent(context.Background(), envelope, botSvc, &fakeDispatchRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 101}})
	if err != nil {
		t.Fatalf("handleAgentMentionEvent returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 for private agent self message", botSvc.chatCalls)
	}
}

func TestHandleAgentMentionEventSkipsPrivateEventWithMissingSenderAndAgentParticipant(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "private",
		SenderID:         0,
		Content:          "旧事件缺 sender，不应让 Agent 猜测并回复自己",
		MsgID:            100,
		ParticipantIDs:   []int64{1001, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}, reply: "reply"}

	err = handleAgentMentionEvent(context.Background(), envelope, botSvc, &fakeDispatchRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 101}})
	if err != nil {
		t.Fatalf("handleAgentMentionEvent returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 for missing sender private agent participant event", botSvc.chatCalls)
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

func TestAgentEventDispatcherSkipsMessageCreatedForMediaMessage(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         1001,
		Content:          `[img]{"id":"3001","name":"error.png","content_type":"image/png"}[/img]`,
		MsgType:          "image",
		MsgID:            99,
		MentionUserIDs:   []int64{2001},
		ParticipantIDs:   []int64{1001, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{
		bot:   &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true},
		reply: "reply",
	}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{}, &fakeAuditRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 101}})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 because file.uploaded event owns media trigger", botSvc.chatCalls)
	}
}

func TestAgentEventDispatcherIgnoresUnmatchedGroupMessage(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         1001,
		Content:          "普通群聊消息，不应该刷屏触发 Agent",
		MsgID:            99,
		ParticipantIDs:   []int64{1001, 1002, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}}
	auditRepo := &fakeAuditRepo{}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{}, auditRepo, &fakeMessageClient{})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0", botSvc.chatCalls)
	}
	if len(auditRepo.records) != 0 {
		t.Fatalf("audit records = %d, want no audit for ignored unmatched group message", len(auditRepo.records))
	}
}

func TestAgentEventDispatcherTriggersConfiguredKeywordRule(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         1001,
		Content:          "线上报错，请分析一下",
		MsgID:            99,
		ParticipantIDs:   []int64{1001, 1002, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{
		bot:   &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true},
		reply: "分析结果",
	}
	msgClient := &fakeMessageClient{
		sendResp: &message.SendMessageResp{Success: true, MsgId: 101},
		historyResp: &message.GetHistoryResp{Success: true, Messages: []*message.Message{
			{SenderId: 1002, Content: "前面提到过 Error 1292", CreatedAt: "2026-05-27 10:00:00"},
		}},
	}
	auditRepo := &fakeAuditRepo{}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: "message.created", Keywords: "报错", TriggerMode: "keyword", Action: "trigger", IsActive: true},
	}}, auditRepo, msgClient)

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 1 {
		t.Fatalf("chat calls = %d, want 1", botSvc.chatCalls)
	}
	if !strings.Contains(botSvc.lastMessage, "前面提到过 Error 1292") {
		t.Fatalf("agent input did not include conversation context: %q", botSvc.lastMessage)
	}
	if msgClient.lastSend == nil || msgClient.lastSend.ClientMsgId == "" {
		t.Fatalf("send request = %#v, want idempotent client msg id", msgClient.lastSend)
	}
	if !auditRepo.hasDecision("trigger") {
		t.Fatalf("audit records = %#v, want trigger audit", auditRepo.records)
	}
}

func TestAgentEventDispatcherRecordsSilentRuleWithoutRunningAgent(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeFileUploaded, "10", events.IMEventPayload{
		EventType:        events.EventTypeFileUploaded,
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         1001,
		ParticipantIDs:   []int64{1001, 2001},
		AttachmentRefs:   []events.AttachmentRef{{FileID: 77, Name: "bug.png", ContentType: "image/png"}},
		IdempotencyKey:   "file.uploaded:77",
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}}
	auditRepo := &fakeAuditRepo{}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: events.EventTypeFileUploaded, TriggerMode: "all", Action: "record", IsActive: true},
	}}, auditRepo, &fakeMessageClient{})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 for silent record", botSvc.chatCalls)
	}
	if len(auditRepo.records) != 1 || auditRepo.records[0].Decision != "record" {
		t.Fatalf("audit records = %#v, want one record decision", auditRepo.records)
	}
}

func TestAgentEventDispatcherInjectsAttachmentContextForFileEvent(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeFileUploaded, "10", events.IMEventPayload{
		EventType:        events.EventTypeFileUploaded,
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         1001,
		ParticipantIDs:   []int64{1001, 2001},
		AttachmentRefs: []events.AttachmentRef{{
			FileID:      77,
			Name:        "error.png",
			ContentType: "image/png",
			URL:         "/files/77",
		}},
		IdempotencyKey: "file.uploaded:77",
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{
		bot:   &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true},
		reply: "收到文件",
	}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: events.EventTypeFileUploaded, TriggerMode: "all", Action: "trigger", IsActive: true},
	}}, &fakeAuditRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 101}})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if !strings.Contains(botSvc.lastMessage, "error.png") || !strings.Contains(botSvc.lastMessage, "/files/77") {
		t.Fatalf("agent input did not include attachment context: %q", botSvc.lastMessage)
	}
	if !strings.Contains(botSvc.lastMessage, "图片OCR") {
		t.Fatalf("agent input did not tell agent to use image OCR tool: %q", botSvc.lastMessage)
	}
}

func TestAgentEventDispatcherHandlesUnifiedMessageReadEvent(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeIMMessageRead, "10", events.IMEventPayload{
		EventType:        events.EventTypeMessageRead,
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         1001,
		MsgID:            99,
		ParticipantIDs:   []int64{1001, 2001},
		IdempotencyKey:   "message.read:1001:99",
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}}
	auditRepo := &fakeAuditRepo{}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: events.EventTypeMessageRead, TriggerMode: "all", Action: "record", IsActive: true},
	}}, auditRepo, &fakeMessageClient{})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want silent record only", botSvc.chatCalls)
	}
	if len(auditRepo.records) != 1 || auditRepo.records[0].EventType != events.EventTypeMessageRead {
		t.Fatalf("audit records = %#v, want message.read record", auditRepo.records)
	}
}

func TestAgentEventDispatcherUsesPayloadIdempotencyKeyForIMEvents(t *testing.T) {
	payload := events.IMEventPayload{
		EventType:        events.EventTypeFileUploaded,
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         1001,
		ParticipantIDs:   []int64{1001, 2001},
		IdempotencyKey:   "file.uploaded:10:77",
		AttachmentRefs:   []events.AttachmentRef{{FileID: 77, Name: "error.png"}},
	}
	first, err := events.NewEnvelope(events.EventTypeFileUploaded, "10", payload)
	if err != nil {
		t.Fatalf("NewEnvelope first returned error: %v", err)
	}
	second, err := events.NewEnvelope(events.EventTypeFileUploaded, "10", payload)
	if err != nil {
		t.Fatalf("NewEnvelope second returned error: %v", err)
	}
	botSvc := &fakeBotService{
		bot:   &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true},
		reply: "收到文件",
	}
	dispatchRepo := &fakeDispatchRepo{seen: map[string]struct{}{}}
	dispatcher := NewAgentEventDispatcher(botSvc, dispatchRepo, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: events.EventTypeFileUploaded, TriggerMode: "all", Action: "trigger", IsActive: true},
	}}, &fakeAuditRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 101}})

	if err := dispatcher.Handle(context.Background(), first); err != nil {
		t.Fatalf("first Handle returned error: %v", err)
	}
	if err := dispatcher.Handle(context.Background(), second); err != nil {
		t.Fatalf("second Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 1 {
		t.Fatalf("chat calls = %d, want one run for duplicate payload idempotency key", botSvc.chatCalls)
	}
}

func TestAgentEventDispatcherIgnoresEventsSentByAnyAgentUser(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         3001,
		Content:          "@agent 我刚才已经回复过了",
		MsgID:            99,
		MentionUserIDs:   []int64{2001},
		ParticipantIDs:   []int64{1001, 2001, 3001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{botsByAgentUserID: map[int64]*model.Bot{
		2001: {ID: 1, AgentUserID: 2001, IsActive: true},
		3001: {ID: 2, AgentUserID: 3001, IsActive: true},
	}}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: events.EventTypeMessageCreated, TriggerMode: "mention", Action: "trigger", IsActive: true},
	}}, &fakeAuditRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 101}})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 for event sent by agent itself", botSvc.chatCalls)
	}
}

func TestAgentEventDispatcherIgnoresAgentGeneratedClientMessage(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         2001,
		Content:          "@agent 这是 Agent 刚写回的消息",
		MsgID:            100,
		MentionUserIDs:   []int64{2001},
		ParticipantIDs:   []int64{1001, 2001},
		ClientMsgID:      "agent:message.created:99:2001",
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: events.EventTypeMessageCreated, TriggerMode: "mention", Action: "trigger", IsActive: true},
	}}, &fakeAuditRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 101}})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 for agent-generated client_msg_id", botSvc.chatCalls)
	}
}

func TestAgentEventDispatcherIgnoresResolvedBotSelfMessageWithoutClientMsgID(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         2001,
		Content:          "@agent 这是 Agent 刚写回但缺少 client_msg_id 的事件",
		MsgID:            101,
		MentionUserIDs:   []int64{2001},
		ParticipantIDs:   []int64{1001, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}}
	audits := &fakeAuditRepo{}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: events.EventTypeMessageCreated, TriggerMode: "mention", Action: "trigger", IsActive: true},
	}}, audits, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 102}})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 for resolved Agent self message without client_msg_id", botSvc.chatCalls)
	}
	if !audits.hasDecision("silent_agent_echo") {
		t.Fatalf("expected silent_agent_echo audit, got %#v", audits.records)
	}
}

func TestAgentEventDispatcherIgnoresPrivateEventWithMissingSenderAndAgentParticipant(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{
		ConversationID:   10,
		ConversationType: "private",
		SenderID:         0,
		Content:          "旧事件 sender 缺失时不能让 Agent 猜测并回复自己",
		MsgID:            102,
		ParticipantIDs:   []int64{1001, 2001},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, nil, &fakeAuditRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 103}})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 for private event with missing sender and agent participant", botSvc.chatCalls)
	}
}

func TestAgentEventDispatcherIgnoresAgentGeneratedIMMetadata(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeFileUploaded, "10", events.IMEventPayload{
		EventType:        events.EventTypeFileUploaded,
		ConversationID:   10,
		ConversationType: "group",
		SenderID:         2001,
		Content:          "@agent 这是 Agent 生成的附件事件",
		MsgID:            102,
		MentionUserIDs:   []int64{2001},
		ParticipantIDs:   []int64{1001, 2001},
		Metadata: map[string]string{
			"agent_generated": "true",
			"client_msg_id":   "agent:message.created:101:2001",
			"source":          "agent",
		},
	})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	botSvc := &fakeBotService{bot: &model.Bot{ID: 1, AgentUserID: 2001, IsActive: true}}
	dispatcher := NewAgentEventDispatcher(botSvc, &fakeDispatchRepo{}, &fakeSubscriptionRepo{rules: []model.AgentSubscriptionRule{
		{BotID: 1, AgentUserID: 2001, ConversationID: 10, EventTypes: events.EventTypeFileUploaded, TriggerMode: "mention", Action: "trigger", IsActive: true},
	}}, &fakeAuditRepo{}, &fakeMessageClient{sendResp: &message.SendMessageResp{Success: true, MsgId: 103}})

	if err := dispatcher.Handle(context.Background(), envelope); err != nil {
		t.Fatalf("dispatcher.Handle returned error: %v", err)
	}
	if botSvc.chatCalls != 0 {
		t.Fatalf("chat calls = %d, want 0 for agent-generated IM metadata", botSvc.chatCalls)
	}
}

type fakeDispatchRepo struct {
	failed string
	seen   map[string]struct{}
}

func (r *fakeDispatchRepo) Start(ctx context.Context, record *model.AgentDispatchRecord) (bool, error) {
	if r.seen != nil {
		key := record.EventID
		if _, ok := r.seen[key]; ok {
			return false, nil
		}
		r.seen[key] = struct{}{}
	}
	return true, nil
}

func (r *fakeDispatchRepo) MarkCompleted(ctx context.Context, eventID string, agentUserID, replyMsgID int64) error {
	return nil
}

func (r *fakeDispatchRepo) MarkFailed(ctx context.Context, eventID string, agentUserID int64, message string) error {
	r.failed = message
	return nil
}

type fakeSubscriptionRepo struct {
	rules []model.AgentSubscriptionRule
}

func (r *fakeSubscriptionRepo) ListActiveRules(ctx context.Context, conversationID int64, eventType string) ([]model.AgentSubscriptionRule, error) {
	var rules []model.AgentSubscriptionRule
	for _, rule := range r.rules {
		if rule.ConversationID != 0 && rule.ConversationID != conversationID {
			continue
		}
		if rule.EventTypes != "" && !strings.Contains(rule.EventTypes, eventType) {
			continue
		}
		if !rule.IsActive {
			continue
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *fakeSubscriptionRepo) UpsertRouteMirror(ctx context.Context, rule *model.AgentSubscriptionRule) error {
	return nil
}

func (r *fakeSubscriptionRepo) DeleteRouteMirror(ctx context.Context, botID int64, routeID int64) error {
	return nil
}

type fakeAuditRepo struct {
	records []model.AgentAuditRecord
}

func (r *fakeAuditRepo) Create(ctx context.Context, record *model.AgentAuditRecord) error {
	if record != nil {
		r.records = append(r.records, *record)
	}
	return nil
}

func (r *fakeAuditRepo) hasDecision(decision string) bool {
	for _, record := range r.records {
		if record.Decision == decision {
			return true
		}
	}
	return false
}

type fakeBotService struct {
	bot               *model.Bot
	botsByAgentUserID map[int64]*model.Bot
	reply             string
	chatErr           error
	chatCalls         int
	lastMessage       string
}

func (s *fakeBotService) CreateBot(context.Context, string, string, string, string, string, string, string, string, string, string, string, string, string, int64, int64, int64, int64, float64, string, bool, string, string, string) (*model.Bot, error) {
	return nil, nil
}
func (s *fakeBotService) UpdateBot(context.Context, int64, int64, string, string, string, string, string, string, string, string, string, string, string, string, bool, bool, int64, int64, int64, float64, string, bool, string, string, string) error {
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
	if s.botsByAgentUserID != nil {
		return s.botsByAgentUserID[agentUserID], nil
	}
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
func (c *fakeMessageClient) TranslateMessage(ctx context.Context, req *message.TranslateMessageReq, callOptions ...callopt.Option) (*message.TranslateMessageResp, error) {
	return &message.TranslateMessageResp{Success: true}, nil
}
