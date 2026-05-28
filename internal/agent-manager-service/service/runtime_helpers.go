package service

import (
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// mergeMessageUsage 是当前包内部使用的方法，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func (u *modelTokenUsage) mergeMessageUsage(msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	usage := msg.ResponseMeta.Usage
	u.Seen = true
	u.InputTokens += int64(usage.PromptTokens)
	u.OutputTokens += int64(usage.CompletionTokens)
}

// botReplyCollector 定义当前包使用的数据结构或接口，用于在业务层、持久化层和传输层之间传递明确语义。
type botReplyCollector struct {
	parts []string
}

// mergeMessage 是当前包内部使用的方法，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
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

// mergeResolvedMessage 是当前包内部使用的方法，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
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

// String 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
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

// needsReplySpace 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
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

// isASCIIWord 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func isASCIIWord(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
