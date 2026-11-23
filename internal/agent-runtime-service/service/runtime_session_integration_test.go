//go:build integration

package service

import (
	"ClaranAIM/internal/agent-runtime-service/store"
	"ClaranAIM/kitex_gen/bot_runtime"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
)

func TestIntegrationRunTaskDoesNotPolluteChatSession(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{assistantEvent("task result")}
	}}
	svc := setupRuntimeIntegrationService(t, agent)
	ctx := context.Background()
	if _, err := svc.RunAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:            testRuntimeBot(),
		UserId:         1,
		ConversationId: 2,
		SessionId:      "it-task-session",
		Input:          "chat turn",
	}); err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	taskResp, err := svc.RunTask(ctx, &bot_runtime.AgentTaskReq{
		Bot:            testRuntimeBot(),
		UserId:         1,
		ConversationId: 2,
		TaskType:       "summary",
		Question:       "总结这段会话",
	})
	if err != nil {
		t.Fatalf("RunTask() error = %v", err)
	}
	if !taskResp.Success || taskResp.Result_ != "task result" {
		t.Fatalf("task resp = %#v", taskResp)
	}
	_, messages, err := svc.sessionStore.Context(ctx, "it-task-session", store.DefaultContextLimit)
	if err != nil {
		t.Fatalf("Context() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("chat session messages = %d, want 2 (task must not persist)", len(messages))
	}
	for _, message := range messages {
		if strings.Contains(message.Content, "总结这段会话") {
			t.Fatal("structured task prompt leaked into chat session")
		}
	}
}

func TestIntegrationSessionSummaryTriggeredAfterManyTurns(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{assistantEvent("short reply")}
	}}
	svc := setupRuntimeIntegrationService(t, agent)
	ctx := context.Background()
	sessionID := "it-summary-session"
	for i := 0; i < 16; i++ {
		if _, err := svc.RunAgent(ctx, &bot_runtime.RunAgentReq{
			Bot:            testRuntimeBot(),
			UserId:         1,
			ConversationId: 2,
			SessionId:      sessionID,
			Input:          fmt.Sprintf("turn %02d with enough text for summary", i),
		}); err != nil {
			t.Fatalf("RunAgent(%d) error = %v", i, err)
		}
	}
	summary, messages, err := svc.sessionStore.Context(ctx, sessionID, store.DefaultContextLimit)
	if err != nil {
		t.Fatalf("Context() error = %v", err)
	}
	if summary == "" {
		t.Fatal("rolling summary should be generated after many turns")
	}
	if len(messages) == 0 || len(messages) > store.DefaultContextLimit {
		t.Fatalf("recent messages = %d, want 1..%d", len(messages), store.DefaultContextLimit)
	}
}

func TestIntegrationGetAgentSessionsFiltersByOwner(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{assistantEvent("ok")}
	}}
	svc := setupRuntimeIntegrationService(t, agent)
	ctx := context.Background()
	for i, userID := range []int64{11, 11, 22} {
		if _, err := svc.RunAgent(ctx, &bot_runtime.RunAgentReq{
			Bot:            testRuntimeBot(),
			UserId:         userID,
			ConversationId: 2,
			SessionId:      fmt.Sprintf("it-owner-session-%d", i),
			Input:          "hello",
		}); err != nil {
			t.Fatalf("RunAgent() error = %v", err)
		}
	}
	resp, err := svc.GetAgentSessions(ctx, &bot_runtime.GetAgentSessionReq{BotId: 7, UserId: 11})
	if err != nil {
		t.Fatalf("GetAgentSessions() error = %v", err)
	}
	if !resp.Success || len(resp.Sessions) != 2 {
		t.Fatalf("sessions for user 11 = %#v, want 2", resp.Sessions)
	}
	for _, session := range resp.Sessions {
		if strings.Contains(session.SessionId, "it-owner-session-2") {
			t.Fatal("session of another user leaked into list")
		}
	}
}

func TestIntegrationListAgentRunsFiltersByConversation(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{assistantEvent("ok")}
	}}
	svc := setupRuntimeIntegrationService(t, agent)
	ctx := context.Background()
	for _, conversationID := range []int64{100, 100, 200} {
		if _, err := svc.RunAgent(ctx, &bot_runtime.RunAgentReq{
			Bot:            testRuntimeBot(),
			UserId:         1,
			ConversationId: conversationID,
			SessionId:      fmt.Sprintf("it-runs-conv-%d", conversationID),
			Input:          "hello",
		}); err != nil {
			t.Fatalf("RunAgent() error = %v", err)
		}
	}
	resp, err := svc.ListAgentRuns(ctx, &bot_runtime.ListAgentRunsReq{UserId: 1, BotId: 7, ConversationId: 100})
	if err != nil {
		t.Fatalf("ListAgentRuns() error = %v", err)
	}
	if !resp.Success || len(resp.Runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(resp.Runs))
	}
	for _, run := range resp.Runs {
		if run.ConversationId != 100 {
			t.Fatalf("run conversation = %d, want 100", run.ConversationId)
		}
	}
}

func TestIntegrationResumeRejectsOtherUser(t *testing.T) {
	agent := &fakeResumableAgent{fakeRunAgent: fakeRunAgent{runFn: func(ctx context.Context, _ *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{interruptEvent(ctx, "write_file", `{"path":"x.txt"}`)}
	}}}
	svc := setupRuntimeIntegrationService(t, agent)
	ctx := context.Background()
	runResp, err := svc.RunAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:            testRuntimeBot(),
		UserId:         1,
		ConversationId: 2,
		SessionId:      "it-resume-owner",
		Input:          "write file",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	resp, err := svc.ResumeAgent(ctx, &bot_runtime.ResumeAgentReq{
		Bot:         testRuntimeBot(),
		UserId:      99,
		RunId:       runResp.GetRunId(),
		InterruptId: runResp.Approval.InterruptId,
		Approved:    true,
	})
	if err != nil {
		t.Fatalf("ResumeAgent() error = %v", err)
	}
	if resp.Success {
		t.Fatal("ResumeAgent() by another user succeeded, want rejection")
	}
	if agent.resumed {
		t.Fatal("agent Resume() must not run for unauthorized user")
	}
}

func TestIntegrationRunAgentRejectsInvalidBotConfig(t *testing.T) {
	agent := &fakeRunAgent{}
	svc := setupRuntimeIntegrationService(t, agent)
	resp, err := svc.RunAgent(context.Background(), &bot_runtime.RunAgentReq{
		Bot:   &bot_runtime.RuntimeBotConfig{BotId: 7},
		Input: "missing credentials",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if resp.Success {
		t.Fatalf("RunAgent() = %#v, want failure", resp)
	}
	runs, err := svc.runStore.ListRuns(context.Background(), store.RunFilter{})
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("invalid config should not create run records, got %d", len(runs))
	}
}
