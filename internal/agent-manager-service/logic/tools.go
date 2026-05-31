package logic

import (
	"context"
	"fmt"
	"strings"
)

// ConversationDigestParams 是会话摘要工具的入参。
type ConversationDigestParams struct {
	Messages string `json:"messages" jsonschema:"description=需要整理的会话文本，可以包含多行发言、文件摘要或引用内容"`
	Focus    string `json:"focus" jsonschema:"description=摘要关注点，例如排障、会议纪要、产品决策、待办提取；可为空"`
}

// BuildConversationDigest 生成稳定格式的会话摘要骨架。
// 该工具不替代模型理解，而是给 Agent 一个可复用的结构化输出模板，适合 IM 会话总结、未读摘要和“我错过了什么”。
func BuildConversationDigest(ctx context.Context, input *ConversationDigestParams) (string, error) {
	_ = ctx
	if input == nil {
		input = &ConversationDigestParams{}
	}
	messages := normalizeText(input.Messages)
	focus := strings.TrimSpace(input.Focus)
	if focus == "" {
		focus = "综合整理"
	}
	if messages == "" {
		messages = "未提供可分析会话内容。"
	}
	return fmt.Sprintf(`## 会话摘要
关注点：%s

%s

## 结论
- 当前内容需要结合上下文判断；如果原始会话缺少明确决策，请标记为“暂无明确结论”。

## 待办
- 从会话中提取负责人、事项和时间；如果没有待办，请写“暂无明确待办”。

## 风险/分歧
- 标出阻塞点、信息缺口、争议点和需要确认的假设。`, focus, bulletizeLines(messages, 8)), nil
}

// TaskBreakdownParams 是任务拆解工具的入参。
type TaskBreakdownParams struct {
	Goal        string `json:"goal" jsonschema:"description=用户希望完成的目标"`
	Constraints string `json:"constraints" jsonschema:"description=时间、权限、环境、依赖、不能做的事等限制条件；可为空"`
}

// BreakDownTask 把用户目标拆成执行清单、验收标准和风险。
func BreakDownTask(ctx context.Context, input *TaskBreakdownParams) (string, error) {
	_ = ctx
	if input == nil {
		input = &TaskBreakdownParams{}
	}
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		goal = "未指定目标"
	}
	constraints := strings.TrimSpace(input.Constraints)
	if constraints == "" {
		constraints = "无额外限制"
	}
	return fmt.Sprintf(`## 任务目标
%s

## 执行步骤
1. 明确输入、期望输出和影响范围。
2. 收集必要上下文：相关会话、文件、接口、错误日志或业务规则。
3. 拆分为可独立验证的小步骤并逐步执行。
4. 完成后给出结果、证据、残余风险和下一步建议。

## 验收标准
- 目标已被直接满足，或明确列出仍缺少什么。
- 每个关键结论都有来源或推理依据。
- 涉及系统变更时说明测试方式和回滚/补救方案。

## 风险
- 限制条件：%s
- 若上下文不足，必须先说明不确定性，再给出保守建议。`, goal, constraints), nil
}

// TextPolishParams 是文本润色工具的入参。
type TextPolishParams struct {
	Text   string `json:"text" jsonschema:"description=需要润色、改写或结构化的原文"`
	Tone   string `json:"tone" jsonschema:"description=目标语气，例如专业、友好、简洁、强硬、安抚；可为空"`
	Format string `json:"format" jsonschema:"description=目标格式，例如通知、回复、邮件、会议纪要、Markdown；可为空"`
}

// PolishText 根据目标语气和格式生成可直接发送的改写结果。
func PolishText(ctx context.Context, input *TextPolishParams) (string, error) {
	_ = ctx
	if input == nil {
		input = &TextPolishParams{}
	}
	text := normalizeText(input.Text)
	if text == "" {
		text = "未提供原文。"
	}
	tone := strings.TrimSpace(input.Tone)
	if tone == "" {
		tone = "清晰、礼貌"
	}
	format := strings.TrimSpace(input.Format)
	if format == "" {
		format = "普通消息"
	}
	return fmt.Sprintf(`## 润色结果
目标语气：%s
目标格式：%s

%s

## 修改说明
- 保留原意，去除含混表达。
- 优先让接收者知道背景、请求和下一步动作。`, tone, format, text), nil
}

func normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func bulletizeLines(text string, limit int) string {
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, "- "+line)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return "- 暂无有效内容。"
	}
	return strings.Join(out, "\n")
}
