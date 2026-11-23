package logic

import (
	"ClaranAIM/kitex_gen/rag"
	"strings"
	"testing"
)

func TestEvaluateGroundingRejectsIrrelevantResults(t *testing.T) {
	verdict := EvaluateGrounding(&rag.SelfRAGCheckpoints{Retrieve: true, IsRel: false, IsSup: true, IsUse: true})
	if verdict.Allowed || !verdict.MustSwitchSource {
		t.Fatalf("verdict = %#v, want blocked with source switch", verdict)
	}
	if !strings.Contains(verdict.Reason, "不相关") {
		t.Fatalf("reason = %q", verdict.Reason)
	}
}

func TestEvaluateGroundingRejectsUnsupportedDraft(t *testing.T) {
	verdict := EvaluateGrounding(&rag.SelfRAGCheckpoints{Retrieve: true, IsRel: true, IsSup: false, IsUse: true})
	if verdict.Allowed || !verdict.MustAskUser || !verdict.MustSwitchSource {
		t.Fatalf("verdict = %#v, want blocked", verdict)
	}
}

func TestEvaluateGroundingRejectsUselessAnswer(t *testing.T) {
	verdict := EvaluateGrounding(&rag.SelfRAGCheckpoints{Retrieve: true, IsRel: true, IsSup: true, IsUse: false})
	if verdict.Allowed || !verdict.MustAskUser {
		t.Fatalf("verdict = %#v, want blocked with ask-user", verdict)
	}
}

func TestEvaluateGroundingAllowsGroundedAnswer(t *testing.T) {
	verdict := EvaluateGrounding(&rag.SelfRAGCheckpoints{Retrieve: true, IsRel: true, IsSup: true, IsUse: true})
	if !verdict.Allowed {
		t.Fatalf("verdict = %#v, want allowed", verdict)
	}
}

func TestEvaluateGroundingWithoutCheckpointIsPermissiveButCautious(t *testing.T) {
	verdict := EvaluateGrounding(nil)
	if !verdict.Allowed {
		t.Fatalf("verdict = %#v, want allowed", verdict)
	}
	if !strings.Contains(verdict.Reason, "核对来源") {
		t.Fatalf("reason = %q", verdict.Reason)
	}
}

func TestGroundingBlockedOutputHidesDraftFromFacts(t *testing.T) {
	verdict := EvaluateGrounding(&rag.SelfRAGCheckpoints{Retrieve: true, IsRel: true, IsSup: false, IsUse: true})
	output := GroundingBlockedOutput(verdict, "ClaranAIM 一定使用了 Redis 做消息队列。")
	if !strings.Contains(output, "未通过") || !strings.Contains(output, "禁止当作事实") {
		t.Fatalf("output = %q", output)
	}
}

func TestAnswerNeedsGroundingRewrite(t *testing.T) {
	cases := []struct {
		answer string
		want   bool
	}{
		{"根据资料可以确定，这个服务一定用了 Kafka。", true},
		{"资料不足，我没有找到能支撑这个结论的依据。", false},
		{"文档明确指出 Outbox 与消息同事务。", true},
		{"正常回答：Outbox 保证消息与事件同事务。", false},
	}
	for _, tc := range cases {
		if got := AnswerNeedsGroundingRewrite(tc.answer); got != tc.want {
			t.Fatalf("AnswerNeedsGroundingRewrite(%q) = %v, want %v", tc.answer, got, tc.want)
		}
	}
}
