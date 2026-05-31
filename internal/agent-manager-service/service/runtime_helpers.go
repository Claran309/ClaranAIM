package service

import (
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// mergeMessageUsage 从 Eino 响应元数据中累加真实 token 用量。
// 如果模型没有返回 usage，Seen 保持 false，计费层会按 usage_missing 记录 0 token。
func (u *modelTokenUsage) mergeMessageUsage(msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	usage := msg.ResponseMeta.Usage
	u.Seen = true
	u.InputTokens += int64(usage.PromptTokens)
	u.OutputTokens += int64(usage.CompletionTokens)
}

// botReplyCollector 收集 Agent 流式输出中的 assistant 文本片段。
// 它会跳过重复片段，避免前端出现“同一句回复连续输出两次”的现象。
type botReplyCollector struct {
	parts []string
}

// mergeMessage 从 ADK MessageVariant 中提取消息并交给 mergeResolvedMessage 过滤角色。
func (r *botReplyCollector) mergeMessage(output *adk.MessageVariant) {
	if output == nil {
		return
	}
	msg, err := output.GetMessage()
	if err != nil {
		return
	}
	r.mergeResolvedMessage(output.Role, msg)
}

// mergeResolvedMessage 只合并 assistant 角色的非空内容。
// 同一片段连续出现时会被丢弃，适配部分流式模型重复发送最终文本的行为。
func (r *botReplyCollector) mergeResolvedMessage(role schema.RoleType, msg *schema.Message) {
	if msg == nil {
		return
	}
	if role != "" && role != schema.Assistant {
		return
	}
	if msg.Role != "" && msg.Role != schema.Assistant {
		return
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return
	}
	if len(r.parts) > 0 && r.parts[len(r.parts)-1] == content {
		return
	}
	r.parts = append(r.parts, content)
}

// String 返回最终可发送给 IM 的 Agent 回复文本。
// 英文单词片段之间会补空格，中文和 Markdown 片段则直接拼接。
func (r *botReplyCollector) String() string {
	if len(r.parts) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, part := range r.parts {
		if sb.Len() > 0 && needsReplySpace(sb.String(), part) {
			sb.WriteByte(' ')
		}
		sb.WriteString(part)
	}
	return strings.TrimSpace(sb.String())
}

// needsReplySpace 判断两个片段之间是否需要补英文空格。
// 只在相邻字符都是 ASCII 字母或数字时补空格，避免破坏中文和 Markdown 标点。
func needsReplySpace(prev, next string) bool {
	if prev == "" || next == "" {
		return false
	}
	lastRunes := []rune(prev)
	nextRunes := []rune(next)
	last := lastRunes[len(lastRunes)-1]
	first := nextRunes[0]
	return isASCIIWord(last) && isASCIIWord(first)
}

// isASCIIWord 判断 rune 是否属于英文单词/数字字符。
func isASCIIWord(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
