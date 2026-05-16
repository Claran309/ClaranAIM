package service

import (
	"math"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
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
