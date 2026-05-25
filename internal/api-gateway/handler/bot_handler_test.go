package handler

import (
	"ClaranAIM/kitex_gen/message"
	"strings"
	"testing"
)

func TestFormatMessagesForAgentContextIncludesVisibleMessages(t *testing.T) {
	got := formatMessagesForAgentContext([]*message.Message{
		{SenderId: 1001, Content: "我们周五前完成接口联调", MsgType: "text", CreatedAt: "2026-05-25 10:00:00"},
		{SenderId: 1002, Content: "我负责前端验收", MsgType: "text", CreatedAt: "2026-05-25 10:01:00"},
	})

	if !strings.Contains(got, "用户1001") || !strings.Contains(got, "周五前完成接口联调") {
		t.Fatalf("context missing first message: %q", got)
	}
	if !strings.Contains(got, "用户1002") || !strings.Contains(got, "我负责前端验收") {
		t.Fatalf("context missing second message: %q", got)
	}
}

func TestMergeAgentQuestionWithContextWarnsWhenNoMessages(t *testing.T) {
	got := mergeAgentQuestionWithContext("总结一下", "")

	if !strings.Contains(got, "未读取到当前用户可见的历史消息") {
		t.Fatalf("empty context warning missing: %q", got)
	}
	if !strings.Contains(got, "总结一下") {
		t.Fatalf("user question missing: %q", got)
	}
}

func TestMergeAgentQuestionWithContextMarksHistoryAsOnlyFactSource(t *testing.T) {
	got := mergeAgentQuestionWithContext("", "- [2026-05-25 10:00:00] 用户1001: 本周上线")

	if !strings.Contains(got, "唯一事实来源") {
		t.Fatalf("context prompt should constrain fact source: %q", got)
	}
	if !strings.Contains(got, "本周上线") {
		t.Fatalf("conversation material missing: %q", got)
	}
}

func TestNewAgentApprovalStoresPendingState(t *testing.T) {
	approval := newAgentApproval(1001, 2002, 3003, "需要确认文件写入")

	if approval.ID == "" {
		t.Fatal("approval id should be generated")
	}
	if approval.UserID != 1001 || approval.BotID != 2002 || approval.ConversationID != 3003 {
		t.Fatalf("approval identifiers not preserved: %#v", approval)
	}
	if approval.Status != agentApprovalStatusPending {
		t.Fatalf("approval status=%q, want pending", approval.Status)
	}
	if !strings.Contains(approval.Description, "文件写入") {
		t.Fatalf("approval description missing: %#v", approval)
	}
}

func TestBuildApprovalConfirmationMessageCarriesUserDecision(t *testing.T) {
	got := buildApprovalConfirmationMessage(agentApproval{
		Description: "需要确认执行文件写入",
	}, "允许写入 README")

	if !strings.Contains(got, "用户已明确允许") {
		t.Fatalf("confirmation message missing approval signal: %q", got)
	}
	if !strings.Contains(got, "允许写入 README") || !strings.Contains(got, "需要确认执行文件写入") {
		t.Fatalf("confirmation message missing details: %q", got)
	}
}
