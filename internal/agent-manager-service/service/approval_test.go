package service

import (
	"ClaranAIM/internal/agent-manager-service/dao"
	"ClaranAIM/internal/agent-manager-service/model"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"
)

type fakeApprovalRepo struct {
	dao.AgentApprovalRepository
	items map[string]*model.AgentApproval
}

func newFakeApprovalRepo(items ...*model.AgentApproval) *fakeApprovalRepo {
	repo := &fakeApprovalRepo{items: map[string]*model.AgentApproval{}}
	for _, item := range items {
		repo.items[item.ApprovalID] = item
	}
	return repo
}

func (r *fakeApprovalRepo) Create(_ context.Context, approval *model.AgentApproval) error {
	if _, exists := r.items[approval.ApprovalID]; exists {
		return errors.New("duplicate approval")
	}
	r.items[approval.ApprovalID] = approval
	return nil
}

func (r *fakeApprovalRepo) Get(_ context.Context, approvalID string) (*model.AgentApproval, error) {
	item, ok := r.items[approvalID]
	if !ok {
		return nil, nil
	}
	copied := *item
	return &copied, nil
}

func (r *fakeApprovalRepo) Update(_ context.Context, approval *model.AgentApproval) error {
	r.items[approval.ApprovalID] = approval
	return nil
}

type fakeBotRepo struct {
	dao.BotRepository
	bot *model.Bot
}

func (r *fakeBotRepo) GetBotByID(context.Context, int64) (*model.Bot, error) {
	return r.bot, nil
}

type fakeBillingRepo struct {
	dao.BillingRepository
	records []model.BillingRecord
}

func (r *fakeBillingRepo) CreateRecord(_ context.Context, record *model.BillingRecord) error {
	r.records = append(r.records, *record)
	return nil
}

type fakeRuntimeResumeClient struct {
	botruntimeservice.Client
	req  *bot_runtime.ResumeAgentReq
	resp *bot_runtime.ResumeAgentResp
}

func (c *fakeRuntimeResumeClient) ResumeAgent(_ context.Context, req *bot_runtime.ResumeAgentReq, _ ...callopt.Option) (*bot_runtime.ResumeAgentResp, error) {
	c.req = req
	return c.resp, nil
}

func testPendingApproval() *model.AgentApproval {
	return &model.AgentApproval{
		ApprovalID:     "apv-1",
		RunID:          "run-1",
		InterruptID:    "tool:1",
		SessionID:      "session-1",
		BotID:          7,
		UserID:         1,
		ConversationID: 2,
		Status:         "pending",
		ToolName:       "write_file",
		Description:    "write a file",
	}
}

func testApprovalService(approval *model.AgentApproval, runtimeResp *bot_runtime.ResumeAgentResp) (*agentServiceImpl, *fakeRuntimeResumeClient, *fakeBillingRepo) {
	approvalRepo := newFakeApprovalRepo(approval)
	billingRepo := &fakeBillingRepo{}
	runtimeClient := &fakeRuntimeResumeClient{resp: runtimeResp}
	svc := &agentServiceImpl{
		botRepo:       &fakeBotRepo{bot: &model.Bot{ID: 7, IsActive: true, ModelName: "glm-4.7", APIKey: "key", BaseURL: "https://example.com/v1"}},
		approvalRepo:  approvalRepo,
		billingRepo:   billingRepo,
		runtimeClient: runtimeClient,
	}
	return svc, runtimeClient, billingRepo
}

func TestResumeAgentApprovesAndCallsRuntime(t *testing.T) {
	approval := testPendingApproval()
	resp := &bot_runtime.ResumeAgentResp{
		Success:   true,
		Reply:     "file written",
		Usage:     &bot_runtime.TokenUsage{InputTokens: 10, OutputTokens: 5, UsageSeen: true},
		RunId:     "run-1",
		SessionId: "session-1",
		Msg:       "ok",
	}
	svc, runtimeClient, billingRepo := testApprovalService(approval, resp)
	result, err := svc.ResumeAgent(context.Background(), "apv-1", 1, true, "allow")
	if err != nil {
		t.Fatalf("ResumeAgent() error = %v", err)
	}
	if result.Reply != "file written" || result.Status != "ok" {
		t.Fatalf("result = %#v", result)
	}
	if runtimeClient.req == nil || !runtimeClient.req.Approved || runtimeClient.req.RunId != "run-1" || runtimeClient.req.InterruptId != "tool:1" {
		t.Fatalf("runtime req = %#v", runtimeClient.req)
	}
	stored := svc.approvalRepo.(*fakeApprovalRepo).items["apv-1"]
	if stored == nil || stored.Status != "approved" || stored.Reason != "allow" {
		t.Fatalf("approval = %#v", stored)
	}
	if len(billingRepo.records) != 1 || billingRepo.records[0].Action != "chat_resume" {
		t.Fatalf("billing = %#v", billingRepo.records)
	}
}

func TestResumeAgentRejectsWithoutExecutionResult(t *testing.T) {
	approval := testPendingApproval()
	resp := &bot_runtime.ResumeAgentResp{Success: true, Reply: "operation rejected", Usage: &bot_runtime.TokenUsage{}, RunId: "run-1", SessionId: "session-1", Msg: "ok"}
	svc, runtimeClient, _ := testApprovalService(approval, resp)
	result, err := svc.ResumeAgent(context.Background(), "apv-1", 1, false, "no")
	if err != nil {
		t.Fatalf("ResumeAgent() error = %v", err)
	}
	if result.Reply != "operation rejected" {
		t.Fatalf("result = %#v", result)
	}
	if runtimeClient.req.Approved || runtimeClient.req.Reason != "no" {
		t.Fatalf("runtime req = %#v", runtimeClient.req)
	}
	stored := svc.approvalRepo.(*fakeApprovalRepo).items["apv-1"]
	if stored.Status != "rejected" {
		t.Fatalf("approval status = %q", stored.Status)
	}
}

func TestResumeAgentRejectsOtherUserAndProcessedRecord(t *testing.T) {
	approval := testPendingApproval()
	svc, _, _ := testApprovalService(approval, &bot_runtime.ResumeAgentResp{Success: true})
	if _, err := svc.ResumeAgent(context.Background(), "apv-1", 2, true, ""); err == nil {
		t.Fatal("ResumeAgent() with other user succeeded, want error")
	}
	approval.Status = "approved"
	if _, err := svc.ResumeAgent(context.Background(), "apv-1", 1, true, ""); err == nil {
		t.Fatal("ResumeAgent() with processed approval succeeded, want error")
	}
}
