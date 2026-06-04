package service

import (
	"ClaranAIM/internal/agent-runtime-service/agent"
	"ClaranAIM/kitex_gen/bot_runtime"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type stubAgent struct {
	reply      string
	interrupt  bool
	interrupts []*adk.InterruptCtx
}

func (a *stubAgent) Name(ctx context.Context) string {
	return "stub"
}

func (a *stubAgent) Description(ctx context.Context) string {
	return "stub agent"
}

func (a *stubAgent) Run(ctx context.Context, input *adk.AgentInput, options ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	if a.interrupt {
		gen.Send(&adk.AgentEvent{Action: &adk.AgentAction{Interrupted: &adk.InterruptInfo{InterruptContexts: a.interrupts}}})
		gen.Close()
		return iter
	}
	gen.Send(adk.EventFromMessage(schema.AssistantMessage(a.reply, nil), nil, schema.Assistant, ""))
	gen.Close()
	return iter
}

func TestGetAgentSessionsNilRequestDoesNotPanic(t *testing.T) {
	svc := NewAgentRuntimeService(RuntimeConfig{})

	resp, err := svc.GetAgentSessions(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetAgentSessions returned error: %v", err)
	}
	if resp == nil || !resp.Success {
		t.Fatalf("GetAgentSessions response = %#v, want success", resp)
	}
}

func TestResolveWorkspaceRootKeepsRelativePathUnderBase(t *testing.T) {
	base := t.TempDir()
	svc := NewAgentRuntimeService(RuntimeConfig{DefaultWorkspaceDir: base}).(*runtimeServiceImpl)

	got, err := svc.resolveWorkspaceRoot(&bot_runtime.RuntimeBotConfig{
		BotId:         49895688258818048,
		WorkspaceRoot: "49895688258818048",
	})
	if err != nil {
		t.Fatalf("resolveWorkspaceRoot returned error: %v", err)
	}
	want := filepath.Join(base, "49895688258818048")
	if got != want {
		t.Fatalf("workspace = %q, want %q", got, want)
	}
}

func TestResolveWorkspaceRootDoesNotDuplicateConfiguredBase(t *testing.T) {
	base := t.TempDir()
	svc := NewAgentRuntimeService(RuntimeConfig{DefaultWorkspaceDir: base}).(*runtimeServiceImpl)

	got, err := svc.resolveWorkspaceRoot(&bot_runtime.RuntimeBotConfig{
		BotId:         49895688258818048,
		WorkspaceRoot: filepath.Join(base, "49895688258818048"),
	})
	if err != nil {
		t.Fatalf("resolveWorkspaceRoot returned error: %v", err)
	}
	want := filepath.Join(base, "49895688258818048")
	if got != want {
		t.Fatalf("workspace = %q, want %q", got, want)
	}
}

func TestResolveWorkspaceRootRepairsLegacyAbsoluteBotIDPath(t *testing.T) {
	base := t.TempDir()
	legacyRoot := t.TempDir()
	svc := NewAgentRuntimeService(RuntimeConfig{DefaultWorkspaceDir: base}).(*runtimeServiceImpl)

	got, err := svc.resolveWorkspaceRoot(&bot_runtime.RuntimeBotConfig{
		BotId:         49895688258818048,
		WorkspaceRoot: filepath.Join(legacyRoot, "49895688258818048"),
	})
	if err != nil {
		t.Fatalf("resolveWorkspaceRoot returned error: %v", err)
	}
	want := filepath.Join(base, "49895688258818048")
	if got != want {
		t.Fatalf("workspace = %q, want repaired path %q", got, want)
	}
}

func TestResolveWorkspaceRootRejectsPathOutsideBase(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Dir(base)
	svc := NewAgentRuntimeService(RuntimeConfig{DefaultWorkspaceDir: base}).(*runtimeServiceImpl)

	_, err := svc.resolveWorkspaceRoot(&bot_runtime.RuntimeBotConfig{
		BotId:         1,
		WorkspaceRoot: outside,
	})
	if err == nil || !strings.Contains(err.Error(), "允许根目录") {
		t.Fatalf("resolveWorkspaceRoot error = %v, want outside-base rejection", err)
	}
}

func TestRuntimeAgentCacheKeyChangesWithProviderConfig(t *testing.T) {
	first := runtimeAgentCacheKey(&bot_runtime.RuntimeBotConfig{
		BotId:         1,
		ModelName:     "glm-4.7",
		ApiKey:        "key-a",
		BaseUrl:       "https://one.example/v1",
		SystemPrompt:  "prompt-a",
		WorkspaceRoot: "storage/agent/files/1",
	})
	second := runtimeAgentCacheKey(&bot_runtime.RuntimeBotConfig{
		BotId:         1,
		ModelName:     "glm-4.7",
		ApiKey:        "key-b",
		BaseUrl:       "https://two.example/v1",
		SystemPrompt:  "prompt-b",
		WorkspaceRoot: "storage/agent/files/1",
	})

	if first == second {
		t.Fatal("cache key should change when runtime provider config changes")
	}
}

func TestRuntimeReplyCollectorJoinsASCIIChunksWithSpace(t *testing.T) {
	var reply replyCollector
	reply.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("hello", nil))
	reply.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("world", nil))

	if got := reply.String(); got != "hello world" {
		t.Fatalf("reply = %q, want %q", got, "hello world")
	}
}

func TestRuntimeReplyCollectorKeepsCJKChunksTight(t *testing.T) {
	var reply replyCollector
	reply.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("你好", nil))
	reply.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("世界", nil))

	if got := reply.String(); got != "你好世界" {
		t.Fatalf("reply = %q, want %q", got, "你好世界")
	}
}

func TestRuntimeReplyCollectorIgnoresToolMessages(t *testing.T) {
	var reply replyCollector
	reply.mergeMessage(&adk.MessageVariant{
		Role:    schema.Tool,
		Message: schema.ToolMessage("tool result", "call-1"),
	})
	reply.mergeMessage(&adk.MessageVariant{
		Role:    schema.Assistant,
		Message: schema.AssistantMessage("final answer", nil),
	})

	if got := reply.String(); got != "final answer" {
		t.Fatalf("reply = %q, want final answer", got)
	}
}

func TestBuildTaskPromptSummaryUsesReadableConversationContext(t *testing.T) {
	got := buildTaskPrompt("summary", "会话材料：用户1说本周上线")

	if strings.Contains(strings.ToLower(got), "json") {
		t.Fatalf("summary prompt should not ask for JSON: %q", got)
	}
	if !strings.Contains(got, "只基于用户提供的“会话材料”") {
		t.Fatalf("summary prompt should require conversation grounding: %q", got)
	}
	if !strings.Contains(got, "不要假装看到了未提供的内容") {
		t.Fatalf("summary prompt should guard against hallucinated context: %q", got)
	}
}

func TestBuildTaskPromptSummaryTreatsSparseChatAsSignal(t *testing.T) {
	got := buildTaskPrompt("summary", "会话材料：\n- [2026-05-25 10:00:00] 用户1: 哈哈\n- [2026-05-25 10:00:01] 用户2: 嗯")

	if strings.Contains(got, "没有足够会话内容") {
		t.Fatalf("summary prompt should not tell agent to refuse sparse chats: %q", got)
	}
	if !strings.Contains(got, "闲聊、灌水、无实质信息") {
		t.Fatalf("summary prompt should ask agent to classify low-value chat: %q", got)
	}
}

func TestToolPolicyInstructionRequiresApprovalForRiskyActions(t *testing.T) {
	got := agent.ToolPolicyInstruction("approval_required")

	if !strings.Contains(got, "等待用户明确同意") {
		t.Fatalf("approval policy should require explicit user consent: %q", got)
	}
	if !strings.Contains(got, "下一轮对话中继续执行") {
		t.Fatalf("approval policy should preserve multi-turn act flow: %q", got)
	}
}

func TestRunTaskDoesNotPersistSummaryIntoLongSession(t *testing.T) {
	svc := NewAgentRuntimeService(RuntimeConfig{SessionDir: t.TempDir()}).(*runtimeServiceImpl)
	svc.agentCache[runtimeAgentCacheKey(&bot_runtime.RuntimeBotConfig{
		BotId:     1,
		ModelName: "test-model",
		ApiKey:    "test-key",
		BaseUrl:   "https://example.invalid/v1",
	})] = &stubAgent{reply: "总结完成"}

	resp, err := svc.RunTask(context.Background(), &bot_runtime.AgentTaskReq{
		Bot: &bot_runtime.RuntimeBotConfig{
			BotId:     1,
			ModelName: "test-model",
			ApiKey:    "test-key",
			BaseUrl:   "https://example.invalid/v1",
		},
		UserId:         2,
		ConversationId: 3,
		TaskType:       "summary",
		Question:       "会话材料：用户1说本周上线",
	})
	if err != nil || resp == nil || !resp.Success {
		t.Fatalf("RunTask resp=%#v err=%v", resp, err)
	}

	session, err := svc.sessionStore.GetSession(defaultSessionID(1, 2, 3))
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if got := len(session.GetMessages()); got != 0 {
		t.Fatalf("summary task should not persist into long session, got %d messages", got)
	}
}

func TestRunAgentReturnsPendingApprovalOnInterrupt(t *testing.T) {
	svc := NewAgentRuntimeService(RuntimeConfig{}).(*runtimeServiceImpl)
	botCfg := &bot_runtime.RuntimeBotConfig{
		BotId:     1,
		ModelName: "test-model",
		ApiKey:    "test-key",
		BaseUrl:   "https://example.invalid/v1",
	}
	svc.agentCache[runtimeAgentCacheKey(botCfg)] = &stubAgent{interrupt: true}

	resp, err := svc.RunAgent(context.Background(), &bot_runtime.RunAgentReq{
		Bot:    botCfg,
		UserId: 2,
		Input:  "删除文件前先问我",
	})
	if err != nil {
		t.Fatalf("RunAgent returned error: %v", err)
	}
	if resp == nil || !resp.Success {
		t.Fatalf("RunAgent resp=%#v, want success pending approval", resp)
	}
	if resp.Msg != "pending_user_approval" {
		t.Fatalf("RunAgent msg=%q, want pending_user_approval", resp.Msg)
	}
	if !strings.Contains(resp.Reply, "需要你确认") {
		t.Fatalf("RunAgent reply=%q, want confirmation prompt", resp.Reply)
	}
}

func TestWithDefaultAgentRunTimeoutAddsTenMinuteDeadline(t *testing.T) {
	ctx, cancel := withDefaultAgentRunTimeout(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("deadline missing, want default Agent run timeout")
	}
	remaining := time.Until(deadline)
	if remaining < 9*time.Minute || remaining > 10*time.Minute {
		t.Fatalf("remaining timeout = %v, want about 10 minutes", remaining)
	}
}

func TestWithDefaultAgentRunTimeoutPreservesExistingDeadline(t *testing.T) {
	base, baseCancel := context.WithTimeout(context.Background(), time.Minute)
	defer baseCancel()
	baseDeadline, _ := base.Deadline()
	ctx, cancel := withDefaultAgentRunTimeout(base)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("deadline missing")
	}
	if !deadline.Equal(baseDeadline) {
		t.Fatalf("deadline = %v, want existing deadline %v", deadline, baseDeadline)
	}
}
