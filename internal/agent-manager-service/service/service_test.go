package service

import (
	"ClaranAIM/internal/agent-manager-service/model"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
	"ClaranAIM/pkg/memoryclient"
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/kitex/client/callopt"
)

func TestModelTokenUsageUsesResponseMetaOnly(t *testing.T) {
	var usage modelTokenUsage
	msg := schema.AssistantMessage("this content must not be estimated", nil)

	usage.mergeMessageUsage(msg)

	if usage.Seen {
		t.Fatal("usage should stay unseen when ResponseMeta.Usage is missing")
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 0 {
		t.Fatalf("missing usage must not be estimated, got input=%d output=%d", usage.InputTokens, usage.OutputTokens)
	}
}

func TestModelTokenUsageMergesActualResponseMeta(t *testing.T) {
	var usage modelTokenUsage

	usage.mergeMessageUsage(&schema.Message{
		ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{
			PromptTokens:     12,
			CompletionTokens: 7,
			TotalTokens:      19,
		}},
	})
	usage.mergeMessageUsage(&schema.Message{
		ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{
			PromptTokens:     3,
			CompletionTokens: 5,
			TotalTokens:      8,
		}},
	})

	if !usage.Seen {
		t.Fatal("usage should be marked seen")
	}
	if usage.InputTokens != 15 || usage.OutputTokens != 12 {
		t.Fatalf("unexpected usage input=%d output=%d", usage.InputTokens, usage.OutputTokens)
	}
}

func TestTokenCostUsesGLM47InputAndOutputRates(t *testing.T) {
	cost := tokenCost("glm-4.7", 1_000_000, 1_000_000)

	if math.Abs(cost-20.44) > 0.000001 {
		t.Fatalf("unexpected glm-4.7 cost: %.6f", cost)
	}
}

func TestSelectBotReplyIgnoresToolAndUsageOnlyMessages(t *testing.T) {
	var reply botReplyCollector
	reply.mergeMessage(&adk.MessageVariant{
		Role:    schema.Tool,
		Message: schema.ToolMessage("tool result", "call-1"),
	})
	reply.mergeMessage(&adk.MessageVariant{
		Role:    schema.Assistant,
		Message: &schema.Message{Role: schema.Assistant, ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 1}}},
	})
	reply.mergeMessage(&adk.MessageVariant{
		Role:    schema.Assistant,
		Message: schema.AssistantMessage("final answer", nil),
	})

	if got := reply.String(); got != "final answer" {
		t.Fatalf("unexpected reply %q", got)
	}
}

func TestSelectBotReplyMergesAssistantTextChunks(t *testing.T) {
	var reply botReplyCollector
	reply.mergeMessage(&adk.MessageVariant{
		Role:    schema.Assistant,
		Message: schema.AssistantMessage("hello", nil),
	})
	reply.mergeMessage(&adk.MessageVariant{
		Role:    schema.Assistant,
		Message: schema.AssistantMessage("world", nil),
	})

	if got := reply.String(); got != "hello world" {
		t.Fatalf("unexpected merged reply %q", got)
	}
}

func TestCreateBotFailsWhenAgentSystemUserCreationFails(t *testing.T) {
	botRepo := &fakeBotRepo{}
	svc := NewAgentService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, &fakeUserClient{
		registerResp: &user.RegisterResp{Success: false, Msg: "user-service unavailable"},
	}, "storage/agent/files")

	created, err := svc.CreateBot(context.Background(), "Agent", "internal", "desc", "glm-4.7", "", "", "", "", "", "", "", "", "", 1001, 0, 0, 0, 0, "", false, "default-key", "https://llm.example/v1", "glm-4.7")
	if err == nil {
		t.Fatalf("CreateBot returned bot=%#v, want error", created)
	}
	if botRepo.created != nil {
		t.Fatalf("bot was persisted despite system user failure: %#v", botRepo.created)
	}
}

func TestCreateBotAppliesDefaultRuntimeSettings(t *testing.T) {
	botRepo := &fakeBotRepo{}
	svc := NewAgentService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, nil, "storage/agent/files")

	created, err := svc.CreateBot(context.Background(), "Agent", "internal", "desc", "glm-4.7", "", "", "", "", "", "", "", "", "", 1001, 0, 0, 0, 0, "", false, "default-key", "https://llm.example/v1", "glm-4.7")
	if err != nil {
		t.Fatalf("CreateBot returned error: %v", err)
	}
	if created.ContextMessageLimit != DefaultContextMessageLimit {
		t.Fatalf("context message limit = %d, want %d", created.ContextMessageLimit, DefaultContextMessageLimit)
	}
	if created.MemoryRecallLimit != DefaultMemoryRecallLimit {
		t.Fatalf("memory recall limit = %d, want %d", created.MemoryRecallLimit, DefaultMemoryRecallLimit)
	}
	if created.GroupTriggerMode != DefaultGroupTriggerMode {
		t.Fatalf("group trigger mode = %q, want %q", created.GroupTriggerMode, DefaultGroupTriggerMode)
	}
	if !created.AutoReplyEnabled {
		t.Fatal("auto reply should be enabled by default")
	}
}

func TestUpdateBotKeepsActiveStateWhenFieldNotSet(t *testing.T) {
	botRepo := &fakeBotRepo{byID: &model.Bot{
		ID:        1,
		Name:      "Agent",
		Type:      "custom",
		ModelName: "glm-4.7",
		APIKey:    "key",
		BaseURL:   "https://llm.example/v1",
		OwnerID:   1001,
		IsActive:  true,
	}}
	svc := NewAgentService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, nil, "storage/agent/files")

	err := svc.UpdateBot(context.Background(), 1, 1001, "New Agent", "", "", "", "", "", "", "", "", "", "", "", false, false, 0, 0, 0, 0, "", false, "default-key", "https://llm.example/v1", "glm-4.7")
	if err != nil {
		t.Fatalf("UpdateBot returned error: %v", err)
	}
	if !botRepo.byID.IsActive {
		t.Fatal("UpdateBot without is_active should keep existing active state")
	}
}

func TestUpdateBotAppliesActiveStateWhenFieldSet(t *testing.T) {
	botRepo := &fakeBotRepo{byID: &model.Bot{
		ID:        1,
		Name:      "Agent",
		Type:      "custom",
		ModelName: "glm-4.7",
		APIKey:    "key",
		BaseURL:   "https://llm.example/v1",
		OwnerID:   1001,
		IsActive:  true,
	}}
	svc := NewAgentService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, nil, "storage/agent/files")

	err := svc.UpdateBot(context.Background(), 1, 1001, "", "", "", "", "", "", "", "", "", "", "", "", false, true, 0, 0, 0, 0, "", false, "default-key", "https://llm.example/v1", "glm-4.7")
	if err != nil {
		t.Fatalf("UpdateBot returned error: %v", err)
	}
	if botRepo.byID.IsActive {
		t.Fatal("UpdateBot with is_active=false should deactivate bot")
	}
}

func TestUpdateBotAppliesRuntimeSettingsWhenProvided(t *testing.T) {
	botRepo := &fakeBotRepo{byID: &model.Bot{
		ID:                  1,
		Name:                "Agent",
		Type:                "custom",
		ModelName:           "glm-4.7",
		APIKey:              "key",
		BaseURL:             "https://llm.example/v1",
		OwnerID:             1001,
		IsActive:            true,
		ContextMessageLimit: DefaultContextMessageLimit,
		MemoryRecallLimit:   DefaultMemoryRecallLimit,
		GroupTriggerMode:    DefaultGroupTriggerMode,
		AutoReplyEnabled:    true,
	}}
	svc := NewAgentService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, nil, "storage/agent/files")

	err := svc.UpdateBot(context.Background(), 1, 1001, "", "", "", "", "", "", "", "", "", "", "", "", true, false, 150, 20, 4096, 0.2, "keyword", false, "default-key", "https://llm.example/v1", "glm-4.7")
	if err != nil {
		t.Fatalf("UpdateBot returned error: %v", err)
	}
	if botRepo.byID.ContextMessageLimit != 150 {
		t.Fatalf("context message limit = %d, want 150", botRepo.byID.ContextMessageLimit)
	}
	if botRepo.byID.MemoryRecallLimit != 20 {
		t.Fatalf("memory recall limit = %d, want 20", botRepo.byID.MemoryRecallLimit)
	}
	if botRepo.byID.MaxOutputTokens != 4096 {
		t.Fatalf("max output tokens = %d, want 4096", botRepo.byID.MaxOutputTokens)
	}
	if botRepo.byID.Temperature != 0.2 {
		t.Fatalf("temperature = %.2f, want 0.2", botRepo.byID.Temperature)
	}
	if botRepo.byID.GroupTriggerMode != "keyword" {
		t.Fatalf("group trigger mode = %q, want keyword", botRepo.byID.GroupTriggerMode)
	}
	if botRepo.byID.AutoReplyEnabled {
		t.Fatal("auto reply should be disabled when explicitly set false")
	}
}

func TestUpdateInternalBotWithUserProviderConvertsToCustom(t *testing.T) {
	botRepo := &fakeBotRepo{byID: &model.Bot{
		ID:        1,
		Name:      "Agent",
		Type:      "internal",
		ModelName: "default-model",
		APIKey:    "default-key",
		BaseURL:   "https://default.example/v1",
		OwnerID:   1001,
		IsActive:  true,
	}}
	svc := NewAgentService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, nil, "storage/agent/files")

	err := svc.UpdateBot(context.Background(), 1, 1001, "", "", "user-model", "user-key", "https://user.example/v1", "", "", "", "", "", "", "", false, false, 0, 0, 0, 0, "", true, "default-key", "https://default.example/v1", "default-model")
	if err != nil {
		t.Fatalf("UpdateBot returned error: %v", err)
	}
	if botRepo.byID.Type != "custom" {
		t.Fatalf("type = %q, want custom", botRepo.byID.Type)
	}
	if botRepo.byID.APIKey != "user-key" || botRepo.byID.BaseURL != "https://user.example/v1" || botRepo.byID.ModelName != "user-model" {
		t.Fatalf("provider config was not updated: %#v", botRepo.byID)
	}
}

func TestCreateRouteMirrorsAgentKeywordRule(t *testing.T) {
	botRepo := &fakeBotRepo{byID: &model.Bot{
		ID:          1,
		Name:        "Agent",
		AgentUserID: 2001,
		OwnerID:     1001,
		IsActive:    true,
	}}
	routeRepo := &fakeRouteRepo{}
	subRepo := &fakeSubscriptionRepo{}
	svc := NewAgentService(botRepo, &fakePermissionRepo{}, routeRepo, &fakeBillingRepo{}, nil, nil, "storage/agent/files")
	svcWithSubscription, ok := svc.(*agentServiceImpl)
	if !ok {
		t.Fatal("NewAgentService should return AgentServiceImpl")
	}
	svcWithSubscription.subscriptionRepo = subRepo

	route, err := svc.CreateRoute(context.Background(), 1, "报错", "agent_keyword", 7)
	if err != nil {
		t.Fatalf("CreateRoute returned error: %v", err)
	}
	if routeRepo.created == nil || routeRepo.created.ID != route.ID {
		t.Fatalf("route was not persisted: %#v", routeRepo.created)
	}
	if subRepo.upserted == nil {
		t.Fatal("expected mirrored subscription rule")
	}
	if subRepo.upserted.AgentUserID != 2001 || subRepo.upserted.TriggerMode != "keyword" || subRepo.upserted.Keywords != "报错" || subRepo.upserted.Action != "trigger" {
		t.Fatalf("unexpected subscription mirror: %#v", subRepo.upserted)
	}
}

func TestCreateRouteMirrorsAgentSilentRecordRule(t *testing.T) {
	botRepo := &fakeBotRepo{byID: &model.Bot{ID: 1, AgentUserID: 2001, OwnerID: 1001, IsActive: true}}
	subRepo := &fakeSubscriptionRepo{}
	svc := NewAgentService(botRepo, &fakePermissionRepo{}, &fakeRouteRepo{}, &fakeBillingRepo{}, nil, nil, "storage/agent/files")
	svc.(*agentServiceImpl).subscriptionRepo = subRepo

	_, err := svc.CreateRoute(context.Background(), 1, "file.uploaded", "agent_record", 1)
	if err != nil {
		t.Fatalf("CreateRoute returned error: %v", err)
	}
	if subRepo.upserted == nil || subRepo.upserted.Action != "record" || !subRepo.upserted.Silent || subRepo.upserted.TriggerMode != "all" {
		t.Fatalf("unexpected silent rule mirror: %#v", subRepo.upserted)
	}
}

func TestDeleteRouteRemovesMirroredSubscriptionRule(t *testing.T) {
	routeRepo := &fakeRouteRepo{created: &model.BotRoute{ID: 22, BotID: 1, RoutePattern: "报错", RouteType: "agent_keyword"}}
	subRepo := &fakeSubscriptionRepo{}
	svc := NewAgentService(&fakeBotRepo{}, &fakePermissionRepo{}, routeRepo, &fakeBillingRepo{}, nil, nil, "storage/agent/files")
	svc.(*agentServiceImpl).subscriptionRepo = subRepo

	if err := svc.DeleteRoute(context.Background(), 22, 1001); err != nil {
		t.Fatalf("DeleteRoute returned error: %v", err)
	}
	if subRepo.deletedRouteID != 22 {
		t.Fatalf("deleted mirrored route id = %d, want 22", subRepo.deletedRouteID)
	}
	if routeRepo.deleted != 22 {
		t.Fatalf("deleted route id = %d, want 22", routeRepo.deleted)
	}
}

func TestChatWithBotInjectsRecalledMemoryAndSkipsLowValueRunSummary(t *testing.T) {
	runtime := &fakeRuntimeClient{}
	memory := &fakeBotMemoryService{
		recallResult: memoryclient.RecallResult{
			Facts: []memoryclient.MemoryFact{{
				BotID:   1,
				UserID:  1001,
				Scope:   memoryclient.ScopeUser,
				Type:    memoryclient.TypePreference,
				Content: "用户喜欢中文短回答",
			}},
			ContextText: "可用长期记忆：\n- [preference/user] 用户喜欢中文短回答",
		},
	}
	svc := NewAgentService(&fakeBotRepo{byID: &model.Bot{
		ID:                  1,
		Name:                "Agent",
		Type:                "custom",
		ModelName:           "glm-4.7",
		APIKey:              "key",
		BaseURL:             "https://llm.example/v1",
		OwnerID:             1001,
		IsActive:            true,
		ContextMessageLimit: 42,
		MemoryRecallLimit:   5,
	}}, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, runtime, nil, "storage/agent/files")
	svc.(*agentServiceImpl).SetMemoryService(memory)

	result, err := svc.ChatWithBot(context.Background(), 1, 1001, 33, "总结一下")
	if err != nil {
		t.Fatalf("ChatWithBot returned error: %v", err)
	}
	if result.Reply != "记住了" {
		t.Fatalf("reply = %q, want runtime reply", result.Reply)
	}
	if runtime.lastReq == nil || !strings.Contains(runtime.lastReq.Input, "用户喜欢中文短回答") || !strings.Contains(runtime.lastReq.Input, "用户本次输入") {
		t.Fatalf("runtime input did not include memory context: %#v", runtime.lastReq)
	}
	if memory.recallInput.BotID != 1 || memory.recallInput.UserID != 1001 || memory.recallInput.ConversationID != 33 {
		t.Fatalf("unexpected recall scope: %#v", memory.recallInput)
	}
	if memory.recallInput.Limit != 5 {
		t.Fatalf("memory recall limit = %d, want 5", memory.recallInput.Limit)
	}
	if runtime.lastReq == nil || runtime.lastReq.Bot == nil || runtime.lastReq.Bot.ContextMessageLimit != 42 {
		t.Fatalf("runtime config did not include context limit: %#v", runtime.lastReq)
	}
	if len(memory.created) != 0 {
		t.Fatalf("expected low-value chat not to create long-term memory, got %#v", memory.created)
	}
}

func TestChatWithBotStoresHighValueRunSummary(t *testing.T) {
	runtime := &fakeRuntimeClient{}
	memory := &fakeBotMemoryService{}
	svc := NewAgentService(&fakeBotRepo{byID: &model.Bot{
		ID:                  1,
		Name:                "Agent",
		Type:                "custom",
		ModelName:           "glm-4.7",
		APIKey:              "key",
		BaseURL:             "https://llm.example/v1",
		OwnerID:             1001,
		IsActive:            true,
		ContextMessageLimit: DefaultContextMessageLimit,
		MemoryRecallLimit:   DefaultMemoryRecallLimit,
	}}, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, runtime, nil, "storage/agent/files")
	svc.(*agentServiceImpl).SetMemoryService(memory)

	_, err := svc.ChatWithBot(context.Background(), 1, 1001, 33, "请长期记住：我正在做 ClaranAIM 的 RAG 和知识图谱模块，偏好先修闭环再打磨 UI")
	if err != nil {
		t.Fatalf("ChatWithBot returned error: %v", err)
	}
	if len(memory.created) != 1 || memory.created[0].Type != memoryclient.TypeAgentRun || memory.created[0].Scope != memoryclient.ScopeConversation {
		t.Fatalf("expected one high-value agent run memory summary, got %#v", memory.created)
	}
}

func TestInternalAgentUsesLatestDefaultProviderAtRuntime(t *testing.T) {
	runtime := &fakeRuntimeClient{}
	svc := NewAgentService(&fakeBotRepo{byID: &model.Bot{
		ID:                  1,
		Name:                "Agent",
		Type:                "internal",
		ModelName:           "old-model",
		APIKey:              "old-key",
		BaseURL:             "https://old.example/v1",
		OwnerID:             1001,
		IsActive:            true,
		ContextMessageLimit: DefaultContextMessageLimit,
		MemoryRecallLimit:   DefaultMemoryRecallLimit,
	}}, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, runtime, nil, "storage/agent/files")
	svc.(*agentServiceImpl).SetDefaultLLM("new-key", "https://new.example/v1", "new-model")

	if _, err := svc.ChatWithBot(context.Background(), 1, 1001, 33, "你好"); err != nil {
		t.Fatalf("ChatWithBot returned error: %v", err)
	}
	if runtime.lastReq == nil || runtime.lastReq.Bot == nil {
		t.Fatalf("runtime request missing: %#v", runtime.lastReq)
	}
	if runtime.lastReq.Bot.ApiKey != "new-key" || runtime.lastReq.Bot.BaseUrl != "https://new.example/v1" {
		t.Fatalf("internal agent did not use latest default provider: %#v", runtime.lastReq.Bot)
	}
	if runtime.lastReq.Bot.ModelName != "old-model" {
		t.Fatalf("explicit model should be kept, got %q", runtime.lastReq.Bot.ModelName)
	}
}

func TestSummarizeAgentRunMemoryUsesTriggeredContentNotSystemEnvelope(t *testing.T) {
	wrapped := "你是 ClaranAIM 中的原生 Agent 成员，本轮输入来自 IM 事件流。\n\n事件信息：\n- event_type: message.created\n\n当前触发内容：\n请长期记住：我正在修 RAG 知识图谱和上传进度问题\n\n会话材料：\n- [2026] 用户1: 历史消息"
	got := summarizeAgentRunMemory(wrapped, "已记录为项目长期问题：RAG 知识图谱和上传进度需要优先闭环。")
	if strings.Contains(got, "你是 ClaranAIM") || strings.Contains(got, "会话材料") {
		t.Fatalf("summary leaked system envelope: %q", got)
	}
	if !strings.Contains(got, "请长期记住") || !strings.Contains(got, "RAG 知识图谱") {
		t.Fatalf("summary = %q, want triggered content and reply", got)
	}
}

type fakeBotRepo struct {
	created *model.Bot
	byID    *model.Bot
}

type fakeRouteRepo struct {
	created *model.BotRoute
	deleted int64
}

func (r *fakeRouteRepo) CreateRoute(ctx context.Context, route *model.BotRoute) error {
	cp := *route
	if cp.ID == 0 {
		cp.ID = 10
		route.ID = cp.ID
	}
	r.created = &cp
	return nil
}

func (r *fakeRouteRepo) GetRouteByID(ctx context.Context, id int64) (*model.BotRoute, error) {
	if r.created != nil && r.created.ID == id {
		return r.created, nil
	}
	return nil, nil
}

func (r *fakeRouteRepo) ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error) {
	return nil, nil
}

func (r *fakeRouteRepo) DeleteRoute(ctx context.Context, id int64) error {
	r.deleted = id
	return nil
}

type fakeSubscriptionRepo struct {
	upserted       *model.AgentSubscriptionRule
	deletedRouteID int64
}

func (r *fakeSubscriptionRepo) ListActiveRules(ctx context.Context, conversationID int64, eventType string) ([]model.AgentSubscriptionRule, error) {
	return nil, nil
}

func (r *fakeSubscriptionRepo) UpsertRouteMirror(ctx context.Context, rule *model.AgentSubscriptionRule) error {
	cp := *rule
	r.upserted = &cp
	return nil
}

func (r *fakeSubscriptionRepo) DeleteRouteMirror(ctx context.Context, botID int64, routeID int64) error {
	r.deletedRouteID = routeID
	return nil
}

func (r *fakeBotRepo) CreateBot(ctx context.Context, bot *model.Bot) error {
	cp := *bot
	if cp.ID == 0 {
		cp.ID = 1
		bot.ID = cp.ID
	}
	r.created = &cp
	r.byID = bot
	return nil
}

func (r *fakeBotRepo) GetBotByID(ctx context.Context, id int64) (*model.Bot, error) {
	if r.byID == nil || r.byID.ID != id {
		return nil, nil
	}
	return r.byID, nil
}

func (r *fakeBotRepo) ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error) {
	return nil, nil
}

func (r *fakeBotRepo) UpdateBot(ctx context.Context, bot *model.Bot) error {
	r.byID = bot
	return nil
}

func (r *fakeBotRepo) DeleteBot(ctx context.Context, id int64) error {
	return nil
}

func (r *fakeBotRepo) GetBotByAgentUserID(ctx context.Context, agentUserID int64) (*model.Bot, error) {
	return nil, nil
}

type fakePermissionRepo struct{}

func (r *fakePermissionRepo) UpsertPermission(ctx context.Context, permission *model.BotPermission) error {
	return nil
}

func (r *fakePermissionRepo) DeletePermission(ctx context.Context, botID, userID int64) error {
	return nil
}

func (r *fakePermissionRepo) GetPermission(ctx context.Context, botID, userID int64) (*model.BotPermission, error) {
	return nil, nil
}

func (r *fakePermissionRepo) ListPermissions(ctx context.Context, botID int64) ([]model.BotPermission, error) {
	return nil, nil
}

type fakeBillingRepo struct{}

func (r *fakeBillingRepo) CreateRecord(ctx context.Context, record *model.BillingRecord) error {
	return nil
}

func (r *fakeBillingRepo) ListRecords(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error) {
	return nil, 0, nil
}

type fakeUserClient struct {
	userservice.Client
	registerResp *user.RegisterResp
	registerErr  error
}

func (c *fakeUserClient) Register(ctx context.Context, req *user.RegisterReq, callOptions ...callopt.Option) (*user.RegisterResp, error) {
	if c.registerErr != nil {
		return nil, c.registerErr
	}
	if c.registerResp != nil {
		return c.registerResp, nil
	}
	return nil, errors.New("not configured")
}

type fakeRuntimeClient struct {
	lastReq *bot_runtime.RunAgentReq
}

func (c *fakeRuntimeClient) RunAgent(ctx context.Context, req *bot_runtime.RunAgentReq, callOptions ...callopt.Option) (*bot_runtime.RunAgentResp, error) {
	c.lastReq = req
	return &bot_runtime.RunAgentResp{
		Success:   true,
		Reply:     "记住了",
		SessionId: req.SessionId,
		Usage:     &bot_runtime.TokenUsage{InputTokens: 3, OutputTokens: 4, UsageSeen: true},
	}, nil
}

func (c *fakeRuntimeClient) SummarizeConversation(ctx context.Context, req *bot_runtime.AgentTaskReq, callOptions ...callopt.Option) (*bot_runtime.AgentTaskResp, error) {
	return nil, errors.New("not implemented")
}

func (c *fakeRuntimeClient) AskConversation(ctx context.Context, req *bot_runtime.AgentTaskReq, callOptions ...callopt.Option) (*bot_runtime.AgentTaskResp, error) {
	return nil, errors.New("not implemented")
}

func (c *fakeRuntimeClient) ExtractInsights(ctx context.Context, req *bot_runtime.AgentTaskReq, callOptions ...callopt.Option) (*bot_runtime.AgentTaskResp, error) {
	return nil, errors.New("not implemented")
}

func (c *fakeRuntimeClient) GenerateReplyCandidates(ctx context.Context, req *bot_runtime.AgentTaskReq, callOptions ...callopt.Option) (*bot_runtime.AgentTaskResp, error) {
	return nil, errors.New("not implemented")
}

func (c *fakeRuntimeClient) GetAgentSessions(ctx context.Context, req *bot_runtime.GetAgentSessionReq, callOptions ...callopt.Option) (*bot_runtime.GetAgentSessionResp, error) {
	return nil, errors.New("not implemented")
}

type fakeBotMemoryService struct {
	recallInput  memoryclient.RecallInput
	recallResult memoryclient.RecallResult
	created      []memoryclient.CreateMemoryInput
}

func (s *fakeBotMemoryService) Recall(ctx context.Context, input memoryclient.RecallInput) (memoryclient.RecallResult, error) {
	s.recallInput = input
	return s.recallResult, nil
}

func (s *fakeBotMemoryService) CreateMemory(ctx context.Context, input memoryclient.CreateMemoryInput) (*memoryclient.MemoryFact, error) {
	s.created = append(s.created, input)
	return &memoryclient.MemoryFact{ID: int64(len(s.created)), BotID: input.BotID, UserID: input.UserID, Scope: input.Scope, Type: input.Type, Content: input.Content}, nil
}

func (s *fakeBotMemoryService) ListMemories(ctx context.Context, viewerID int64, filter memoryclient.Filter) ([]memoryclient.MemoryFact, int64, error) {
	out := make([]memoryclient.MemoryFact, 0, len(s.created))
	for i, item := range s.created {
		out = append(out, memoryclient.MemoryFact{
			ID:             int64(i + 1),
			BotID:          item.BotID,
			UserID:         item.UserID,
			ConversationID: item.ConversationID,
			SessionID:      item.SessionID,
			Scope:          item.Scope,
			Type:           item.Type,
			Content:        item.Content,
			Enabled:        true,
		})
	}
	return out, int64(len(out)), nil
}
