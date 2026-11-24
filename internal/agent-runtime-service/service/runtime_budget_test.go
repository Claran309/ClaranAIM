package service

import (
	"context"
	"strings"
	"testing"

	runtimestore "ClaranAIM/internal/agent-runtime-service/store"
	"ClaranAIM/kitex_gen/bot_runtime"

	commontool "github.com/cloudwego/eino-examples/adk/common/tool"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func assistantEventWithUsage(content string, inputTokens, outputTokens int) *adk.AgentEvent {
	msg := schema.AssistantMessage(content, nil)
	msg.ResponseMeta = &schema.ResponseMeta{
		Usage: &schema.TokenUsage{PromptTokens: inputTokens, CompletionTokens: outputTokens},
	}
	return &adk.AgentEvent{Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
		Role:    schema.Assistant,
		Message: msg,
	}}}
}

func TestBudgetTrackerEnforcesAllLimits(t *testing.T) {
	usage := tokenUsage{input: 120, output: 80, seen: true}
	cases := []struct {
		name   string
		budget RunBudget
		steps  int64
		want   string
	}{
		{"steps", RunBudget{MaxSteps: 2, MaxInputTokens: 1000, MaxOutputTokens: 1000, MaxCostUSD: 100}, 3, "max_steps"},
		{"input", RunBudget{MaxSteps: 10, MaxInputTokens: 100, MaxOutputTokens: 1000, MaxCostUSD: 100}, 1, "max_input_tokens"},
		{"output", RunBudget{MaxSteps: 10, MaxInputTokens: 1000, MaxOutputTokens: 50, MaxCostUSD: 100}, 1, "max_output_tokens"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := newBudgetTracker(tc.budget, "glm-4.7")
			tracker.steps = tc.steps
			if got := tracker.exceeded(usage); got != tc.want {
				t.Fatalf("exceeded() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunAgentStopsAtMaxStepsAndReturnsPartialReply(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{
			assistantEvent("first part"),
			assistantEvent("second part"),
			assistantEvent("third part"),
		}
	}}
	svc := testRuntimeService(t, agent, "")
	resp, err := svc.RunAgent(context.Background(), &bot_runtime.RunAgentReq{
		Bot:    testRuntimeBot(),
		UserId: 1,
		Input:  "run too many steps",
		Budget: &bot_runtime.RunBudget{MaxSteps: 1},
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Msg, "budget_exceeded:max_steps") {
		t.Fatalf("response = %#v", resp)
	}
	if !strings.Contains(resp.Reply, "first part") || !strings.Contains(resp.Reply, "运行已中断") {
		t.Fatalf("partial reply = %q", resp.Reply)
	}
	runs, err := svc.runStore.ListRuns(context.Background(), runtimestore.RunFilter{})
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "budget_exceeded" || runs[0].InterruptReason != "max_steps" {
		t.Fatalf("runs = %#v", runs)
	}
}

func TestRunAgentStopsAtInputTokenBudget(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{assistantEventWithUsage("expensive", 500, 10)}
	}}
	svc := testRuntimeService(t, agent, "")
	resp, err := svc.RunAgent(context.Background(), &bot_runtime.RunAgentReq{
		Bot:    testRuntimeBot(),
		UserId: 1,
		Input:  "budget",
		Budget: &bot_runtime.RunBudget{MaxInputTokens: 100},
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if !strings.Contains(resp.Msg, "budget_exceeded:max_input_tokens") {
		t.Fatalf("response msg = %q", resp.Msg)
	}
}

func TestBuildApprovalInfoRedactsSecrets(t *testing.T) {
	info := &adk.InterruptInfo{InterruptContexts: []*adk.InterruptCtx{{
		ID: "tool:approval-1",
		Info: &commontool.ApprovalInfo{
			ToolName:        "write_file",
			ArgumentsInJSON: `{"path":"a.txt","api_key":"super-secret","nested":{"password":"p"}}`,
		},
	}}}
	got := buildApprovalInfo("run-1", "session-1", 7, 1, 2, info)
	if got.ApprovalId == "" || got.InterruptId != "tool:approval-1" || got.ToolName != "write_file" {
		t.Fatalf("approval = %#v", got)
	}
	if strings.Contains(got.ArgumentsJson, "super-secret") || strings.Contains(got.ArgumentsJson, `"p"`) {
		t.Fatalf("sensitive arguments not redacted: %s", got.ArgumentsJson)
	}
	if !strings.Contains(got.ArgumentsJson, "[REDACTED]") {
		t.Fatalf("redaction marker missing: %s", got.ArgumentsJson)
	}
}
