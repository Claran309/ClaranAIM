package service

import (
	"ClaranAIM/internal/bot-manager-service/model"
	memorymodel "ClaranAIM/internal/memory-service/model"
	memorysvc "ClaranAIM/internal/memory-service/service"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
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
	svc := NewBotService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, &fakeUserClient{
		registerResp: &user.RegisterResp{Success: false, Msg: "user-service unavailable"},
	}, "storage/agent/workspaces")

	created, err := svc.CreateBot(context.Background(), "Agent", "internal", "desc", "glm-4.7", "", "", "", "", "", "", "", "", "", 1001, "default-key", "https://llm.example/v1", "glm-4.7")
	if err == nil {
		t.Fatalf("CreateBot returned bot=%#v, want error", created)
	}
	if botRepo.created != nil {
		t.Fatalf("bot was persisted despite system user failure: %#v", botRepo.created)
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
	svc := NewBotService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, nil, "storage/agent/workspaces")

	err := svc.UpdateBot(context.Background(), 1, 1001, "New Agent", "", "", "", "", "", "", "", "", "", "", "", false, false, "default-key", "https://llm.example/v1", "glm-4.7")
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
	svc := NewBotService(botRepo, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, nil, nil, "storage/agent/workspaces")

	err := svc.UpdateBot(context.Background(), 1, 1001, "", "", "", "", "", "", "", "", "", "", "", "", false, true, "default-key", "https://llm.example/v1", "glm-4.7")
	if err != nil {
		t.Fatalf("UpdateBot returned error: %v", err)
	}
	if botRepo.byID.IsActive {
		t.Fatal("UpdateBot with is_active=false should deactivate bot")
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
	svc := NewBotService(botRepo, &fakePermissionRepo{}, routeRepo, &fakeBillingRepo{}, nil, nil, "storage/agent/workspaces")
	svcWithSubscription, ok := svc.(*botServiceImpl)
	if !ok {
		t.Fatal("NewBotService should return botServiceImpl")
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
	svc := NewBotService(botRepo, &fakePermissionRepo{}, &fakeRouteRepo{}, &fakeBillingRepo{}, nil, nil, "storage/agent/workspaces")
	svc.(*botServiceImpl).subscriptionRepo = subRepo

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
	svc := NewBotService(&fakeBotRepo{}, &fakePermissionRepo{}, routeRepo, &fakeBillingRepo{}, nil, nil, "storage/agent/workspaces")
	svc.(*botServiceImpl).subscriptionRepo = subRepo

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

func TestChatWithBotInjectsRecalledMemoryAndStoresRunSummary(t *testing.T) {
	runtime := &fakeRuntimeClient{}
	memory := &fakeBotMemoryService{
		recallResult: memorysvc.RecallResult{
			Facts: []memorymodel.MemoryFact{{
				BotID:   1,
				UserID:  1001,
				Scope:   memorymodel.ScopeUser,
				Type:    memorymodel.TypePreference,
				Content: "用户喜欢中文短回答",
			}},
			ContextText: "可用长期记忆：\n- [preference/user] 用户喜欢中文短回答",
		},
	}
	svc := NewBotService(&fakeBotRepo{byID: &model.Bot{
		ID:        1,
		Name:      "Agent",
		Type:      "custom",
		ModelName: "glm-4.7",
		APIKey:    "key",
		BaseURL:   "https://llm.example/v1",
		OwnerID:   1001,
		IsActive:  true,
	}}, &fakePermissionRepo{}, nil, &fakeBillingRepo{}, runtime, nil, "storage/agent/workspaces")
	svc.(*botServiceImpl).SetMemoryService(memory)

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
	if len(memory.created) != 1 || memory.created[0].Type != memorymodel.TypeAgentRun || memory.created[0].Scope != memorymodel.ScopeConversation {
		t.Fatalf("expected one agent run memory summary, got %#v", memory.created)
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
	recallInput  memorysvc.RecallInput
	recallResult memorysvc.RecallResult
	created      []memorysvc.CreateMemoryInput
}

func (s *fakeBotMemoryService) Recall(ctx context.Context, input memorysvc.RecallInput) (memorysvc.RecallResult, error) {
	s.recallInput = input
	return s.recallResult, nil
}

func (s *fakeBotMemoryService) CreateMemory(ctx context.Context, input memorysvc.CreateMemoryInput) (*memorymodel.MemoryFact, error) {
	s.created = append(s.created, input)
	return &memorymodel.MemoryFact{ID: int64(len(s.created)), BotID: input.BotID, UserID: input.UserID, Scope: input.Scope, Type: input.Type, Content: input.Content}, nil
}
