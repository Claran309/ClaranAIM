package service

import (
	"ClaranAIM/internal/agent-runtime-service/store"
	"ClaranAIM/kitex_gen/bot_runtime"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type fakeRunAgent struct {
	runFn func(ctx context.Context, input *adk.AgentInput) []*adk.AgentEvent
}

func (a *fakeRunAgent) Name(context.Context) string        { return "fake-agent" }
func (a *fakeRunAgent) Description(context.Context) string { return "fake agent for runtime tests" }

func (a *fakeRunAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()
		for _, event := range a.runFn(ctx, input) {
			generator.Send(event)
		}
	}()
	return iterator
}

func testRuntimeBot() *bot_runtime.RuntimeBotConfig {
	return &bot_runtime.RuntimeBotConfig{
		BotId:     7,
		ApiKey:    "test-key",
		BaseUrl:   "https://example.com/v1",
		ModelName: "test-model",
	}
}

func testRuntimeService(t *testing.T, agent adk.Agent, storeDir string) *runtimeServiceImpl {
	t.Helper()
	var sessionStore store.SessionStore
	if storeDir != "" {
		jsonlStore, err := store.NewJSONLStore(storeDir)
		if err != nil {
			t.Fatalf("NewJSONLStore() error = %v", err)
		}
		sessionStore = jsonlStore
	}
	runtimeStore := store.NewMemoryRuntimeStore()
	bot := testRuntimeBot()
	return &runtimeServiceImpl{
		sessionStore:    sessionStore,
		runStore:        runtimeStore,
		checkpointStore: runtimeStore,
		agentCache:      map[string]adk.Agent{runtimeAgentCacheKey(bot): agent},
	}
}

func assistantEvent(content string) *adk.AgentEvent {
	return &adk.AgentEvent{
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Role:    schema.Assistant,
				Message: schema.AssistantMessage(content, nil),
			},
		},
	}
}

func TestRunAgentRestoresSessionAndPersistsReply(t *testing.T) {
	dir := t.TempDir()
	sessionStore, err := store.NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	owner := store.SessionOwner{BotID: 7, UserID: 1}
	if err := sessionStore.AppendMessage(context.Background(), "session-restore", owner, "user", "old question", 0); err != nil {
		t.Fatalf("append old user error = %v", err)
	}
	if err := sessionStore.AppendMessage(context.Background(), "session-restore", owner, "assistant", "old answer", 0); err != nil {
		t.Fatalf("append old assistant error = %v", err)
	}

	var captured []adk.Message
	agent := &fakeRunAgent{runFn: func(_ context.Context, input *adk.AgentInput) []*adk.AgentEvent {
		captured = append([]adk.Message(nil), input.Messages...)
		return []*adk.AgentEvent{assistantEvent("new answer")}
	}}
	svc := testRuntimeService(t, agent, dir)

	resp, err := svc.RunAgent(context.Background(), &bot_runtime.RunAgentReq{
		Bot:       testRuntimeBot(),
		UserId:    1,
		SessionId: "session-restore",
		Input:     "new question",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if !resp.Success || resp.Reply != "new answer" {
		t.Fatalf("RunAgent() response = %#v", resp)
	}
	if len(captured) != 3 {
		t.Fatalf("captured messages = %d, want 2 history + 1 input", len(captured))
	}
	if captured[len(captured)-1].Content != "new question" {
		t.Fatalf("last captured message = %q", captured[len(captured)-1].Content)
	}

	reloaded, err := store.NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore(reload) error = %v", err)
	}
	session, err := reloaded.GetSession(context.Background(), "session-restore")
	if err != nil {
		t.Fatalf("GetSession(reload) error = %v", err)
	}
	messages := session.Messages
	if len(messages) != 4 {
		t.Fatalf("persisted messages = %d, want 4", len(messages))
	}
	if messages[2].Content != "new question" || messages[3].Content != "new answer" {
		t.Fatalf("persisted tail = %#v", messages[2:])
	}
}

func TestRunAgentReturnsFailureWhenContextTimesOut(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(ctx context.Context, _ *adk.AgentInput) []*adk.AgentEvent {
		<-ctx.Done()
		return []*adk.AgentEvent{{Err: ctx.Err()}}
	}}
	svc := testRuntimeService(t, agent, "")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	resp, err := svc.RunAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:   testRuntimeBot(),
		Input: "timeout",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if resp.Success {
		t.Fatal("RunAgent() should fail after context timeout")
	}
	if !strings.Contains(resp.Msg, "Agent执行失败") {
		t.Fatalf("failure message = %q", resp.Msg)
	}
}

func TestRunAgentMapsInterruptToPendingApproval(t *testing.T) {
	agent := &fakeRunAgent{runFn: func(context.Context, *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{{
			Action: &adk.AgentAction{
				Interrupted: &adk.InterruptInfo{
					InterruptContexts: make([]*adk.InterruptCtx, 2),
				},
			},
		}}
	}}
	svc := testRuntimeService(t, agent, "")

	resp, err := svc.RunAgent(context.Background(), &bot_runtime.RunAgentReq{
		Bot:       testRuntimeBot(),
		SessionId: "session-approval",
		Input:     "execute something",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if !resp.Success || resp.Msg != "pending_user_approval" {
		t.Fatalf("RunAgent() response = %#v", resp)
	}
	if !strings.Contains(resp.Reply, "待确认步骤数：2") {
		t.Fatalf("approval reply = %q", resp.Reply)
	}
}
