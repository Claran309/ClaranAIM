package reflection

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeCritic struct {
	content string
	err     error
	delay   time.Duration
	usage   *schema.TokenUsage
}

func (f *fakeCritic) Generate(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	msg := schema.AssistantMessage(f.content, nil)
	if f.usage != nil {
		msg.ResponseMeta = &schema.ResponseMeta{Usage: f.usage}
	}
	return msg, nil
}

func (f *fakeCritic) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not implemented")
}

func testConfig(critic model.BaseChatModel) Config {
	return Config{
		Enabled:   true,
		ModelName: "glm-4.7",
		ChatModel: critic,
		Budget: Budget{
			MaxRounds:      1,
			MaxTokens:      1200,
			Timeout:        2 * time.Second,
			MinAnswerRunes: 10,
		},
	}
}

func TestReflectKeepsAnswerWhenCriticPasses(t *testing.T) {
	critic := &fakeCritic{content: `{"complete":true,"grounded":true,"issues":[],"revised_answer":""}`}
	result := Reflect(context.Background(), testConfig(critic), Evidence{Task: "q"}, "这是一个足够长的正确答案，有来源支撑。")
	if !result.Called || !result.Passed || result.Changed {
		t.Fatalf("result = %#v", result)
	}
	if result.Answer != "这是一个足够长的正确答案，有来源支撑。" {
		t.Fatalf("answer changed unexpectedly: %q", result.Answer)
	}
}

func TestReflectRewritesWhenNotGrounded(t *testing.T) {
	critic := &fakeCritic{content: `{"complete":true,"grounded":false,"issues":["没有引用来源"],"revised_answer":"资料不足，无法确认该结论。"}`}
	result := Reflect(context.Background(), testConfig(critic), Evidence{Task: "q"}, "这是一个足够长的原答案，但它没有来源支撑。")
	if !result.Called || result.Passed || !result.Changed {
		t.Fatalf("result = %#v", result)
	}
	if result.Answer != "资料不足，无法确认该结论。" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if len(result.Issues) != 1 || result.Issues[0] != "没有引用来源" {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestReflectSkipsWhenDisabledOrTooShort(t *testing.T) {
	cfg := testConfig(&fakeCritic{content: "{}"})
	cfg.Enabled = false
	if result := Reflect(context.Background(), cfg, Evidence{}, "短"); result.Called || result.SkippedReason != "disabled" {
		t.Fatalf("disabled result = %#v", result)
	}
	cfg.Enabled = true
	if result := Reflect(context.Background(), cfg, Evidence{}, "短"); result.Called || result.SkippedReason != "answer_too_short" {
		t.Fatalf("short result = %#v", result)
	}
}

func TestReflectKeepsOriginalOnInvalidJSON(t *testing.T) {
	critic := &fakeCritic{content: "not a json"}
	result := Reflect(context.Background(), testConfig(critic), Evidence{}, "这是一个足够长的原答案，需要保留。")
	if !result.Called || !result.Passed || result.Changed {
		t.Fatalf("result = %#v", result)
	}
	if !strings.HasPrefix(result.SkippedReason, "critic_invalid_json") {
		t.Fatalf("skip reason = %q", result.SkippedReason)
	}
}

func TestReflectKeepsOriginalOnTimeout(t *testing.T) {
	cfg := testConfig(&fakeCritic{content: "{}", delay: 300 * time.Millisecond})
	cfg.Budget.Timeout = 30 * time.Millisecond
	result := Reflect(context.Background(), cfg, Evidence{}, "这是一个足够长的原答案，超时后必须保留。")
	if result.Answer != "这是一个足够长的原答案，超时后必须保留。" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !strings.HasPrefix(result.SkippedReason, "critic_error") {
		t.Fatalf("skip reason = %q", result.SkippedReason)
	}
}

func TestReflectSkipsWhenTokenBudgetExceeded(t *testing.T) {
	critic := &fakeCritic{
		content: `{"complete":true,"grounded":true,"issues":[],"revised_answer":""}`,
		usage:   &schema.TokenUsage{PromptTokens: 100, CompletionTokens: 5000},
	}
	cfg := testConfig(critic)
	cfg.Budget.MaxTokens = 100
	result := Reflect(context.Background(), cfg, Evidence{}, "这是一个足够长的原答案，预算超限后保留。")
	if result.Changed || result.SkippedReason != "critic_budget_exceeded" {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseCriticOutputToleratesMarkdownFence(t *testing.T) {
	out, err := parseCriticOutput("```json\n{\"complete\":false,\"grounded\":false,\"issues\":[\"x\"],\"revised_answer\":\"y\"}\n```")
	if err != nil {
		t.Fatalf("parseCriticOutput() error = %v", err)
	}
	if out.Complete || out.Grounded || out.RevisedAnswer != "y" {
		t.Fatalf("out = %#v", out)
	}
}

func TestReflectSummaryHasNoAnswerBody(t *testing.T) {
	critic := &fakeCritic{content: `{"complete":true,"grounded":true,"issues":[],"revised_answer":""}`}
	result := Reflect(context.Background(), testConfig(critic), Evidence{}, "这是一个足够长的原答案，摘要里不应该出现正文。")
	summary := result.Summary()
	if strings.Contains(summary, "这是一个足够长的原答案") {
		t.Fatalf("summary leaked answer body: %q", summary)
	}
	if !strings.Contains(summary, "called=true") {
		t.Fatalf("summary = %q", summary)
	}
}
