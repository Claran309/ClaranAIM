package logic

import (
	"context"
	"strings"
	"testing"
)

func TestBuildConversationDigestProducesGeneralIMSummary(t *testing.T) {
	out, err := BuildConversationDigest(context.Background(), &ConversationDigestParams{
		Messages: "张三: 登录失败\n李四: 我排查数据库\n王五: 下午前给结论",
		Focus:    "排障",
	})
	if err != nil {
		t.Fatalf("BuildConversationDigest returned error: %v", err)
	}
	for _, want := range []string{"会话摘要", "结论", "待办", "风险"} {
		if !strings.Contains(out, want) {
			t.Fatalf("digest missing %q: %s", want, out)
		}
	}
	for _, legacy := range []string{"罗德岛", "干员", "作战"} {
		if strings.Contains(out, legacy) {
			t.Fatalf("digest contains legacy term %q: %s", legacy, out)
		}
	}
}

func TestBreakDownTaskProducesChecklist(t *testing.T) {
	out, err := BreakDownTask(context.Background(), &TaskBreakdownParams{
		Goal:        "修复登录失败",
		Constraints: "今天内完成",
	})
	if err != nil {
		t.Fatalf("BreakDownTask returned error: %v", err)
	}
	for _, want := range []string{"执行步骤", "验收标准", "风险"} {
		if !strings.Contains(out, want) {
			t.Fatalf("task breakdown missing %q: %s", want, out)
		}
	}
}

func TestPolishTextSupportsToneAndFormat(t *testing.T) {
	out, err := PolishText(context.Background(), &TextPolishParams{
		Text:   "这个接口挂了你看看",
		Tone:   "专业",
		Format: "通知",
	})
	if err != nil {
		t.Fatalf("PolishText returned error: %v", err)
	}
	if !strings.Contains(out, "润色结果") || !strings.Contains(out, "专业") || !strings.Contains(out, "通知") {
		t.Fatalf("unexpected polish result: %s", out)
	}
}
