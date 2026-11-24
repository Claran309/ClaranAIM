//go:build integration

package service

import (
	"ClaranAIM/internal/agent-runtime-service/store"
	"ClaranAIM/kitex_gen/bot_runtime"
	"context"
	"os"
	"testing"

	commontool "github.com/cloudwego/eino-examples/adk/common/tool"
	"github.com/cloudwego/eino/adk"
)

func interruptEvent(ctx context.Context, toolName, args string) *adk.AgentEvent {
	return adk.Interrupt(ctx, &commontool.ApprovalInfo{ToolName: toolName, ArgumentsInJSON: args})
}

const runtimeIntegrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupRuntimeIntegrationService(t *testing.T, agent adk.Agent) *runtimeServiceImpl {
	t.Helper()
	dsn := os.Getenv(runtimeIntegrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 runtime MySQL 集成测试", runtimeIntegrationDSNEnv)
	}
	db, err := store.InitDB(dsn)
	if err != nil {
		t.Fatalf("store.InitDB() error = %v", err)
	}
	for _, table := range []string{"agent_messages", "agent_sessions", "agent_runs", "agent_checkpoints"} {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clean table %s error = %v", table, err)
		}
	}
	gormStore := store.NewGormStore(db)
	bot := testRuntimeBot()
	return &runtimeServiceImpl{
		sessionStore:    gormStore,
		runStore:        gormStore,
		checkpointStore: gormStore,
		agentCache:      map[string]adk.Agent{runtimeAgentCacheKey(bot): agent},
	}
}

func TestIntegrationRunAgentPersistsSessionAndRunToMySQL(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{assistantEventWithUsage("mysql persisted answer", 30, 10)}
	}}
	svc := setupRuntimeIntegrationService(t, agent)
	ctx := context.Background()
	resp, err := svc.RunAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:            testRuntimeBot(),
		UserId:         1,
		ConversationId: 2,
		SessionId:      "it-runtime-session",
		Input:          "persist me",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if !resp.Success || resp.GetRunId() == "" {
		t.Fatalf("resp = %#v", resp)
	}
	summary, messages, err := svc.sessionStore.Context(ctx, "it-runtime-session", store.DefaultContextLimit)
	if err != nil {
		t.Fatalf("Context() error = %v", err)
	}
	if summary != "" {
		t.Fatalf("unexpected summary = %q", summary)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("messages = %#v", messages)
	}
	run, err := svc.runStore.GetRun(ctx, resp.GetRunId())
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run == nil || run.Status != store.RunStatusCompleted || run.InputTokens != 30 || run.OutputTokens != 10 {
		t.Fatalf("run = %#v", run)
	}
	if run.Step != 1 {
		t.Fatalf("step = %d, want 1", run.Step)
	}
}

func TestIntegrationRunAgentPersistsBudgetExceededRun(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{
			assistantEvent("step one"),
			assistantEvent("step two"),
			assistantEvent("step three"),
		}
	}}
	svc := setupRuntimeIntegrationService(t, agent)
	resp, err := svc.RunAgent(context.Background(), &bot_runtime.RunAgentReq{
		Bot:            testRuntimeBot(),
		UserId:         1,
		ConversationId: 2,
		SessionId:      "it-runtime-budget",
		Input:          "too many steps",
		Budget:         &bot_runtime.RunBudget{MaxSteps: 1},
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	run, err := svc.runStore.GetRun(context.Background(), resp.GetRunId())
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run == nil || run.Status != store.RunStatusBudgetExceeded || run.InterruptReason != "max_steps" {
		t.Fatalf("run = %#v", run)
	}
}

func TestIntegrationApprovalRunPersistsCheckpointAndResumes(t *testing.T) {
	agent := &fakeResumableAgent{fakeRunAgent: fakeRunAgent{runFn: func(ctx context.Context, _ *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{interruptEvent(ctx, "write_file", `{"path":"it.txt"}`)}
	}}}
	svc := setupRuntimeIntegrationService(t, agent)
	ctx := context.Background()
	runResp, err := svc.RunAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:            testRuntimeBot(),
		UserId:         1,
		ConversationId: 2,
		SessionId:      "it-runtime-approval",
		Input:          "write a file",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if runResp.Approval == nil {
		t.Fatalf("resp = %#v", runResp)
	}
	pending, err := svc.runStore.GetRun(ctx, runResp.GetRunId())
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if pending.Status != store.RunStatusPendingApproval || pending.ApprovalID != runResp.Approval.ApprovalId {
		t.Fatalf("pending run = %#v", pending)
	}
	if pending.CheckpointJSON == "" {
		t.Fatal("checkpoint json should record interrupt/approval ids")
	}
	if _, ok, err := svc.checkpointStore.Get(ctx, runResp.GetRunId()); err != nil || !ok {
		t.Fatalf("Eino checkpoint missing: ok=%v err=%v", ok, err)
	}
	resumeResp, err := svc.ResumeAgent(ctx, &bot_runtime.ResumeAgentReq{
		Bot:         testRuntimeBot(),
		UserId:      1,
		RunId:       runResp.GetRunId(),
		InterruptId: runResp.Approval.InterruptId,
		ApprovalId:  runResp.Approval.ApprovalId,
		Approved:    true,
	})
	if err != nil {
		t.Fatalf("ResumeAgent() error = %v", err)
	}
	if !resumeResp.Success {
		t.Fatalf("resume resp = %#v", resumeResp)
	}
	completed, err := svc.runStore.GetRun(ctx, runResp.GetRunId())
	if err != nil {
		t.Fatalf("GetRun(after resume) error = %v", err)
	}
	if completed.Status != store.RunStatusCompleted {
		t.Fatalf("completed run = %#v", completed)
	}
}

func TestIntegrationResumeRejectsUnknownRun(t *testing.T) {
	agent := &fakeResumableAgent{}
	svc := setupRuntimeIntegrationService(t, agent)
	resp, err := svc.ResumeAgent(context.Background(), &bot_runtime.ResumeAgentReq{
		Bot:         testRuntimeBot(),
		UserId:      1,
		RunId:       "missing-run",
		InterruptId: "tool:1",
	})
	if err != nil {
		t.Fatalf("ResumeAgent() error = %v", err)
	}
	if resp.Success {
		t.Fatalf("resume unknown run = %#v, want failure", resp)
	}
}
