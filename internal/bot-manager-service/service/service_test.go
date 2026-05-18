package service

import (
	"ClaranAIM/internal/bot-manager-service/model"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
	"context"
	"errors"
	"math"
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

type fakeBotRepo struct {
	created *model.Bot
	byID    *model.Bot
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
