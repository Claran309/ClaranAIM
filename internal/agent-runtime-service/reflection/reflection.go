// Package reflection 实现 Agent 最终输出后的单轮 critic 自检。
//
// 设计约束（来自优化清单 R4）：
//   - critic 只输出 JSON，最多 1 轮改写；
//   - 高风险工具在 critic 确认前不算完成；
//   - 失败、超时、预算不足时保留原稿，不让 Reflection 成为新的故障源。
package reflection

import (
	"ClaranAIM/pkg/agentkit/pricing"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Budget 是 Reflection 的独立预算，不与 Agent 工具步数共享。
type Budget struct {
	MaxRounds      int
	MaxTokens      int64
	Timeout        time.Duration
	MaxCostUSD     float64
	MinAnswerRunes int
}

// Config 描述 critic 使用的模型和开关。
type Config struct {
	Enabled   bool
	ModelName string
	ChatModel model.BaseChatModel
	Budget    Budget
}

// Result 是一次 Reflection 的完整结果，用于写入 agent_runs 和 OTel。
type Result struct {
	Called        bool
	Changed       bool
	Passed        bool
	Rounds        int
	Issues        []string
	Answer        string
	SkippedReason string
	InputTokens   int64
	OutputTokens  int64
	CostUSD       float64
	LatencyMS     int64
}

// CriticOutput 是 critic 必须返回的 JSON 结构。
type CriticOutput struct {
	Complete      bool     `json:"complete"`
	Grounded      bool     `json:"grounded"`
	Issues        []string `json:"issues"`
	RevisedAnswer string   `json:"revised_answer"`
}

// Evidence 是 critic 判断“是否有依据”的输入。
type Evidence struct {
	ToolCalls   []string
	Sources     []string
	HasHighRisk bool
	Task        string
}

const criticSystemPrompt = `你是 Agent 输出的 critic。只输出 JSON，不要输出解释或 Markdown。

判断标准：
1. complete：是否真正完成了用户任务，而不是只描述计划。
2. grounded：结论是否有工具结果或来源支撑；没有证据时必须为 false。
3. issues：列出具体问题，最多 5 条；没有问题时为空数组。
4. revised_answer：只有 complete=false 或 grounded=false 时才给出改写后的完整答案；
   否则返回空字符串。

要求：
- 不确定是否完成时判为 false。
- 不要因为措辞不好就改写；只在事实、完整性、引用依据有问题时改写。
- 改写后的答案不能编造来源；资料不足就明确说明资料不足。

输出格式：
{"complete":true,"grounded":true,"issues":[],"revised_answer":""}`

// Reflect 执行最多 1 轮 critic 自检。
// 任何失败都返回原稿并把原因写入 SkippedReason，调用方不应把它当致命错误。
func Reflect(ctx context.Context, cfg Config, evidence Evidence, answer string) Result {
	result := Result{Answer: answer, Passed: true}
	if !cfg.Enabled || cfg.ChatModel == nil {
		result.SkippedReason = "disabled"
		return result
	}
	if strings.TrimSpace(answer) == "" {
		result.SkippedReason = "empty_answer"
		return result
	}
	if cfg.Budget.MinAnswerRunes > 0 && len([]rune(answer)) < cfg.Budget.MinAnswerRunes {
		result.SkippedReason = "answer_too_short"
		return result
	}
	timeout := cfg.Budget.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	result.Called = true
	messages := []*schema.Message{
		schema.SystemMessage(criticSystemPrompt),
		schema.UserMessage(buildCriticPrompt(evidence, answer)),
	}
	msg, err := cfg.ChatModel.Generate(runCtx, messages)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.SkippedReason = "critic_error:" + err.Error()
		return result
	}
	if msg == nil {
		result.SkippedReason = "critic_empty_response"
		return result
	}
	if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		result.InputTokens = int64(msg.ResponseMeta.Usage.PromptTokens)
		result.OutputTokens = int64(msg.ResponseMeta.Usage.CompletionTokens)
		result.CostUSD = pricing.EstimateUSD(cfg.ModelName, result.InputTokens, result.OutputTokens)
	}
	if cfg.Budget.MaxTokens > 0 && result.OutputTokens > cfg.Budget.MaxTokens {
		result.SkippedReason = "critic_budget_exceeded"
		return result
	}
	if cfg.Budget.MaxCostUSD > 0 && result.CostUSD > cfg.Budget.MaxCostUSD {
		result.SkippedReason = "critic_cost_exceeded"
		return result
	}
	parsed, parseErr := parseCriticOutput(msg.Content)
	if parseErr != nil {
		result.SkippedReason = "critic_invalid_json:" + parseErr.Error()
		return result
	}
	result.Rounds = 1
	result.Issues = parsed.Issues
	result.Passed = parsed.Complete && parsed.Grounded
	if result.Passed {
		return result
	}
	revised := strings.TrimSpace(parsed.RevisedAnswer)
	if revised == "" {
		result.SkippedReason = "critic_no_revision"
		return result
	}
	result.Answer = revised
	result.Changed = revised != strings.TrimSpace(answer)
	return result
}

// parseCriticOutput 容忍模型在 JSON 外包裹 Markdown 代码块或说明文字。
func parseCriticOutput(content string) (CriticOutput, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return CriticOutput{}, errors.New("no json object")
	}
	var out CriticOutput
	if err := json.Unmarshal([]byte(content[start:end+1]), &out); err != nil {
		return CriticOutput{}, err
	}
	if out.Issues == nil {
		out.Issues = []string{}
	}
	return out, nil
}

func buildCriticPrompt(evidence Evidence, answer string) string {
	var b strings.Builder
	b.WriteString("用户任务：\n")
	b.WriteString(defaultString(strings.TrimSpace(evidence.Task), "（未提供）"))
	b.WriteString("\n\n工具调用摘要：\n")
	if len(evidence.ToolCalls) == 0 {
		b.WriteString("- 无\n")
	} else {
		for _, call := range evidence.ToolCalls {
			b.WriteString("- ")
			b.WriteString(truncate(call, 300))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n可用来源：\n")
	if len(evidence.Sources) == 0 {
		b.WriteString("- 无\n")
	} else {
		for _, src := range evidence.Sources {
			b.WriteString("- ")
			b.WriteString(truncate(src, 200))
			b.WriteString("\n")
		}
	}
	if evidence.HasHighRisk {
		b.WriteString("\n注意：本轮执行过高风险工具（写文件/执行命令/发消息）。")
		b.WriteString("除非工具结果明确显示成功，否则 complete 必须为 false。\n")
	}
	b.WriteString("\n最终答案：\n")
	b.WriteString(answer)
	return b.String()
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// Summary 返回一行可写入日志/运行记录的摘要，不包含答案正文。
func (r Result) Summary() string {
	return fmt.Sprintf("called=%t passed=%t changed=%t rounds=%d issues=%d skip=%s latency_ms=%d tokens=%d/%d cost_usd=%.6f",
		r.Called, r.Passed, r.Changed, r.Rounds, len(r.Issues), r.SkippedReason,
		r.LatencyMS, r.InputTokens, r.OutputTokens, r.CostUSD)
}
