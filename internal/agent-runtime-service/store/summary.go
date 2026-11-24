package store

import (
	"strings"
)

// buildRollingSummary 在旧摘要基础上追加新消息的紧凑摘要。
// 这里使用确定性文本摘要，不额外调用模型，避免摘要本身突破运行预算。
func buildRollingSummary(previous string, messages []Message, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = DefaultSummaryMaxRunes
	}
	var b strings.Builder
	previous = strings.TrimSpace(previous)
	if previous != "" {
		b.WriteString(previous)
		b.WriteString("\n")
	}
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(strings.TrimSpace(msg.Role))
		b.WriteString(": ")
		b.WriteString(truncateRunes(content, 160))
		b.WriteString("\n")
	}
	return truncateFromStart(b.String(), maxRunes)
}

// makeSessionTitle 从首条用户消息生成可读标题。
func makeSessionTitle(content string) string {
	title := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if title == "" {
		return "新会话"
	}
	return truncateRunes(title, 60)
}

// truncateRunes 按字符截断，避免中文被截成半个 UTF-8。
func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

// truncateFromStart 保留摘要末尾，丢弃最早内容并显式标注折叠。
func truncateFromStart(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	const marker = "（更早摘要已折叠）\n"
	keep := limit - len([]rune(marker))
	if keep <= 0 {
		return string(runes[len(runes)-limit:])
	}
	return marker + string(runes[len(runes)-keep:])
}
