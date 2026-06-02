package handler

import (
	"ClaranAIM/pkg/memoryclient"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

type fakeMemoryService struct {
	createInput    memoryclient.CreateMemoryInput
	candidateInput memoryclient.CandidateInput
}

func (f *fakeMemoryService) CreateMemory(ctx context.Context, input memoryclient.CreateMemoryInput) (*memoryclient.MemoryFact, error) {
	_ = ctx
	f.createInput = input
	return &memoryclient.MemoryFact{ID: 1, BotID: input.BotID, UserID: input.UserID, OwnerUserID: input.OwnerUserID, Scope: input.Scope, Type: input.Type, Content: input.Content, Enabled: true}, nil
}

func (f *fakeMemoryService) ListMemories(ctx context.Context, viewerID int64, filter memoryclient.Filter) ([]memoryclient.MemoryFact, int64, error) {
	_ = ctx
	_ = viewerID
	_ = filter
	return nil, 0, nil
}

func (f *fakeMemoryService) Recall(ctx context.Context, input memoryclient.RecallInput) (memoryclient.RecallResult, error) {
	_ = ctx
	_ = input
	return memoryclient.RecallResult{}, nil
}

func (f *fakeMemoryService) UpdateMemory(ctx context.Context, viewerID, memoryID int64, input memoryclient.UpdateMemoryInput) (*memoryclient.MemoryFact, error) {
	_ = ctx
	_ = viewerID
	_ = memoryID
	_ = input
	return &memoryclient.MemoryFact{}, nil
}

func (f *fakeMemoryService) DeleteMemory(ctx context.Context, viewerID, memoryID int64) error {
	_ = ctx
	_ = viewerID
	_ = memoryID
	return nil
}

func (f *fakeMemoryService) CreateCandidate(ctx context.Context, input memoryclient.CandidateInput) (*memoryclient.MemoryCandidate, error) {
	_ = ctx
	f.candidateInput = input
	return &memoryclient.MemoryCandidate{ID: 1, BotID: input.BotID, UserID: input.UserID, OwnerUserID: input.OwnerUserID, Scope: input.Scope, Type: input.Type, Content: input.Content, Status: "pending"}, nil
}

func (f *fakeMemoryService) ListCandidates(ctx context.Context, viewerID int64, filter memoryclient.CandidateFilter) ([]memoryclient.MemoryCandidate, int64, error) {
	_ = ctx
	_ = viewerID
	_ = filter
	return nil, 0, nil
}

func (f *fakeMemoryService) AcceptCandidate(ctx context.Context, viewerID, candidateID int64) (*memoryclient.MemoryCandidate, error) {
	_ = ctx
	_ = viewerID
	_ = candidateID
	return &memoryclient.MemoryCandidate{}, nil
}

func (f *fakeMemoryService) RejectCandidate(ctx context.Context, viewerID, candidateID int64) (*memoryclient.MemoryCandidate, error) {
	_ = ctx
	_ = viewerID
	_ = candidateID
	return &memoryclient.MemoryCandidate{}, nil
}

func TestCreateMemoryAllowsSystemIMNativeBotIDZero(t *testing.T) {
	fake := &fakeMemoryService{}
	h := &MemoryHandler{svc: fake}
	c := app.NewContext(0)
	c.Set("userID", int64(1001))
	body, _ := json.Marshal(map[string]interface{}{
		"bot_id":  0,
		"scope":   "conversation",
		"type":    "chat_summary",
		"title":   "会话摘要",
		"content": "这是 IM 原生会话沉淀，不绑定单个 Agent。",
	})
	c.Request.SetMethod(http.MethodPost)
	c.Request.SetBody(body)

	h.CreateMemory(context.Background(), c)

	if c.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
	if fake.createInput.BotID != 0 {
		t.Fatalf("bot id = %d, want 0", fake.createInput.BotID)
	}
	if fake.createInput.UserID != 1001 || fake.createInput.OwnerUserID != 1001 {
		t.Fatalf("user ownership not derived from auth context: %#v", fake.createInput)
	}
}

func TestCreateCandidateAllowsSystemIMNativeBotIDZero(t *testing.T) {
	fake := &fakeMemoryService{}
	h := &MemoryHandler{svc: fake}
	c := app.NewContext(0)
	c.Set("userID", int64(1001))
	body, _ := json.Marshal(map[string]interface{}{
		"bot_id":  0,
		"scope":   "user",
		"type":    "preference",
		"title":   "候选记忆",
		"content": "用户希望先确认再写入长期记忆。",
		"source":  "conversation-intelligence",
	})
	c.Request.SetMethod(http.MethodPost)
	c.Request.SetBody(body)

	h.CreateCandidate(context.Background(), c)

	if c.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
	if fake.candidateInput.BotID != 0 {
		t.Fatalf("candidate bot id = %d, want 0", fake.candidateInput.BotID)
	}
	if fake.candidateInput.UserID != 1001 || fake.candidateInput.OwnerUserID != 1001 {
		t.Fatalf("candidate ownership not derived from auth context: %#v", fake.candidateInput)
	}
}
