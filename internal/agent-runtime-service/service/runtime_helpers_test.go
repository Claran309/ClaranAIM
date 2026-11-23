package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ClaranAIM/kitex_gen/bot_runtime"

	"github.com/cloudwego/eino/schema"
)

func TestWithDefaultAgentRunTimeoutUsesTenMinutes(t *testing.T) {
	ctx, cancel := withDefaultAgentRunTimeout(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("withDefaultAgentRunTimeout() returned context without deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 9*time.Minute+50*time.Second || remaining > 10*time.Minute+2*time.Second {
		t.Fatalf("remaining timeout = %v, want about 10m", remaining)
	}
}

func TestWithDefaultAgentRunTimeoutKeepsExistingDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer parentCancel()
	parentDeadline, _ := parent.Deadline()

	ctx, cancel := withDefaultAgentRunTimeout(parent)
	defer cancel()
	got, ok := ctx.Deadline()
	if !ok || !got.Equal(parentDeadline) {
		t.Fatalf("deadline = %v (ok=%v), want preserved %v", got, ok, parentDeadline)
	}
}

func TestWithDefaultAgentRunTimeoutHandlesNilContext(t *testing.T) {
	ctx, cancel := withDefaultAgentRunTimeout(nil)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("nil context should still receive a default deadline")
	}
}

func TestTrimRuntimeSessionHistoryKeepsLatestMessages(t *testing.T) {
	messages := make([]*schema.Message, 15)
	for i := range messages {
		messages[i] = schema.UserMessage(strings.Repeat("x", i+1))
	}

	got := trimRuntimeSessionHistory(messages, 12)
	if len(got) != 12 {
		t.Fatalf("trimmed length = %d, want 12", len(got))
	}
	if got[0] != messages[3] || got[len(got)-1] != messages[len(messages)-1] {
		t.Fatal("trimRuntimeSessionHistory did not keep the latest window")
	}

	if short := trimRuntimeSessionHistory(messages[:3], 12); len(short) != 3 {
		t.Fatalf("short history length = %d, want 3", len(short))
	}
	if zero := trimRuntimeSessionHistory(messages, 0); len(zero) != len(messages) {
		t.Fatalf("limit=0 length = %d, want %d", len(zero), len(messages))
	}
}

func TestIsTransientAgentRunError(t *testing.T) {
	transient := []string{
		"unexpected EOF",
		"connection reset by peer",
		"connection refused",
		"server closed idle connection",
		"tls handshake timeout",
	}
	for _, text := range transient {
		if !isTransientAgentRunError(errors.New(text)) {
			t.Fatalf("isTransientAgentRunError(%q) = false, want true", text)
		}
	}

	permanent := []string{"invalid api key", "model not found", "rate limit exceeded: 429"}
	for _, text := range permanent {
		if isTransientAgentRunError(errors.New(text)) {
			t.Fatalf("isTransientAgentRunError(%q) = true, want false", text)
		}
	}
	if isTransientAgentRunError(nil) {
		t.Fatal("nil error must not be transient")
	}
}

func TestReplyCollectorDeduplicatesAndSpacesEnglish(t *testing.T) {
	var collector replyCollector
	collector.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("Hello", nil))
	collector.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("Hello", nil))
	collector.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("world", nil))
	collector.mergeResolvedMessage(schema.User, schema.UserMessage("ignored"))

	if got := collector.String(); got != "Hello world" {
		t.Fatalf("String() = %q, want %q", got, "Hello world")
	}
}

func TestReplyCollectorKeepsChineseWithoutSpaces(t *testing.T) {
	var collector replyCollector
	collector.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("你好", nil))
	collector.mergeResolvedMessage(schema.Assistant, schema.AssistantMessage("世界", nil))

	if got := collector.String(); got != "你好世界" {
		t.Fatalf("String() = %q, want 你好世界", got)
	}
}

func TestTokenUsageMergesAndConverts(t *testing.T) {
	var usage tokenUsage
	msg := &schema.Message{
		Role: schema.Assistant,
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 12, CompletionTokens: 7},
		},
	}
	usage.mergeMessageUsage(msg)
	usage.mergeMessageUsage(nil)

	got := usage.toRPC()
	if got.InputTokens != 12 || got.OutputTokens != 7 || !got.UsageSeen {
		t.Fatalf("toRPC() = %#v, want 12/7 seen", got)
	}
}

func TestValidateRuntimeBot(t *testing.T) {
	valid := &bot_runtime.RuntimeBotConfig{BotId: 1, ApiKey: "key", BaseUrl: "https://example.com/v1", ModelName: "model"}
	if err := validateRuntimeBot(valid); err != nil {
		t.Fatalf("validateRuntimeBot(valid) error = %v", err)
	}

	cases := []struct {
		name string
		bot  *bot_runtime.RuntimeBotConfig
		want string
	}{
		{"missing bot id", &bot_runtime.RuntimeBotConfig{}, "bot_id"},
		{"missing key", &bot_runtime.RuntimeBotConfig{BotId: 1}, "API Key"},
		{"missing base url", &bot_runtime.RuntimeBotConfig{BotId: 1, ApiKey: "key"}, "Base URL"},
		{"missing model", &bot_runtime.RuntimeBotConfig{BotId: 1, ApiKey: "key", BaseUrl: "url"}, "模型"},
	}
	for _, tc := range cases {
		err := validateRuntimeBot(tc.bot)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: error = %v, want contains %q", tc.name, err, tc.want)
		}
	}
}

func TestDefaultSessionIDIncludesScopes(t *testing.T) {
	if got := defaultSessionID(7, 8, 9); got != "agent_7_user_8_conv_9" {
		t.Fatalf("defaultSessionID() = %q", got)
	}
}
