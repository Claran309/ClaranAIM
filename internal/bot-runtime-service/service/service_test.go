package service

import (
	"ClaranAIM/kitex_gen/bot_runtime"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestGetAgentSessionsNilRequestDoesNotPanic(t *testing.T) {
	svc := NewBotRuntimeService(RuntimeConfig{})

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
	svc := NewBotRuntimeService(RuntimeConfig{DefaultWorkspaceDir: base}).(*runtimeServiceImpl)

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

func TestResolveWorkspaceRootRejectsPathOutsideBase(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Dir(base)
	svc := NewBotRuntimeService(RuntimeConfig{DefaultWorkspaceDir: base}).(*runtimeServiceImpl)

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
		WorkspaceRoot: "storage/agent/workspaces/1",
	})
	second := runtimeAgentCacheKey(&bot_runtime.RuntimeBotConfig{
		BotId:         1,
		ModelName:     "glm-4.7",
		ApiKey:        "key-b",
		BaseUrl:       "https://two.example/v1",
		SystemPrompt:  "prompt-b",
		WorkspaceRoot: "storage/agent/workspaces/1",
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
