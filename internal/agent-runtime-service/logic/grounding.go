package logic

import (
	"ClaranAIM/kitex_gen/rag"
	"strings"
)

// GroundingVerdict 是 Self-RAG 检查点对 Agent 最终答案的门禁结论。
// R5 的目标不是“打印指标”，而是决定 Agent 能不能把检索草稿当作确定答案。
type GroundingVerdict struct {
	// Allowed 表示本轮检索结果可以作为确定答案使用。
	Allowed bool
	// MustAskUser 表示资料不足或与问题不相关，应追问或换关键词。
	MustAskUser bool
	// MustSwitchSource 表示内部资料不够，应改用 web_search 或其他来源。
	MustSwitchSource bool
	// Reason 是给模型看的门禁说明。
	Reason string
}

// EvaluateGrounding 把 Self-RAG 的 IsRel/IsSup/IsUse 转成可执行门禁。
//
// 规则：
//   - IsRel=false：检索结果与问题不相关，必须丢弃本轮草稿，换关键词重查。
//   - IsSup=false：资料不足以支撑答案，禁止把草稿当确定结论，必须说明资料不足或追问。
//   - IsUse=false：答案对用户没有直接帮助，应追问需求或改用其他来源。
func EvaluateGrounding(check *rag.SelfRAGCheckpoints) GroundingVerdict {
	if check == nil {
		return GroundingVerdict{
			Allowed:     true,
			MustAskUser: false,
			Reason:      "本轮没有 Self-RAG 检查点，按普通检索结果使用，但仍需自行核对来源。",
		}
	}
	if !check.GetIsRel() {
		return GroundingVerdict{
			Allowed:          false,
			MustAskUser:      true,
			MustSwitchSource: true,
			Reason:           "Self-RAG 判定检索结果与问题不相关（IsRel=false）：本轮来源不可用，请换关键词重新检索，或向用户澄清问题范围。",
		}
	}
	if !check.GetIsSup() {
		return GroundingVerdict{
			Allowed:          false,
			MustAskUser:      true,
			MustSwitchSource: true,
			Reason:           "Self-RAG 判定资料不足以支撑答案（IsSup=false）：禁止把“答案草稿”当作确定结论。请明确说明资料不足，或追问用户补充信息，或改用 web_search。",
		}
	}
	if !check.GetIsUse() {
		return GroundingVerdict{
			Allowed:     false,
			MustAskUser: true,
			Reason:      "Self-RAG 判定答案对用户帮助有限（IsUse=false）：请先追问用户真正想要的目标，再决定是否继续检索。",
		}
	}
	return GroundingVerdict{Allowed: true, Reason: "Self-RAG 通过：检索结果相关、可支撑答案且对用户有用。"}
}

// GroundingBlockedOutput 是门禁拦截时返回给模型的工具输出。
// 它显式覆盖“答案草稿”，避免模型把草稿直接抄进最终回复。
func GroundingBlockedOutput(verdict GroundingVerdict, draft string) string {
	var b strings.Builder
	b.WriteString("## 知识库检索门禁：未通过\n")
	b.WriteString("- 结论：")
	b.WriteString(verdict.Reason)
	b.WriteString("\n")
	if verdict.MustSwitchSource {
		b.WriteString("- 下一步：换关键词重查，或调用 web_search，或在最终回复中明确说明“资料不足”。\n")
	}
	if verdict.MustAskUser {
		b.WriteString("- 下一步：如果无法补充资料，必须向用户追问，不要编造答案。\n")
	}
	if strings.TrimSpace(draft) != "" {
		b.WriteString("\n## 不可直接引用的草稿（仅供参考，禁止当作事实输出）\n")
		b.WriteString(strings.TrimSpace(draft))
		b.WriteString("\n")
	}
	return b.String()
}

// 常见“资料不足却硬答”的措辞，用于最终答案兜底检查。
var ungroundedClaimMarkers = []string{
	"根据资料可以确定",
	"根据知识库可以确定",
	"文档明确指出",
	"资料明确说明",
	"一定是因为",
	"肯定是",
}

// 允许的“资料不足”表达，出现这些措辞时不触发兜底改写。
var insufficientEvidenceMarkers = []string{
	"资料不足",
	"没有找到",
	"未找到",
	"知识库中没有",
	"无法确认",
	"无法确定",
	"不确定",
	"缺少依据",
	"请补充",
	"建议补充",
	"需要更多信息",
}

// AnswerNeedsGroundingRewrite 判断最终答案是否在“没有依据”的情况下给出确定性结论。
func AnswerNeedsGroundingRewrite(answer string) bool {
	text := strings.ToLower(strings.TrimSpace(answer))
	if text == "" {
		return false
	}
	for _, marker := range insufficientEvidenceMarkers {
		if strings.Contains(text, marker) {
			return false
		}
	}
	for _, marker := range ungroundedClaimMarkers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
