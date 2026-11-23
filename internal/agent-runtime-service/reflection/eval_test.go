package reflection

import (
	"ClaranAIM/pkg/agentkit/component"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

const defaultEvalTimeout = 5 * time.Second

func evalUsage() *schema.TokenUsage {
	return &schema.TokenUsage{PromptTokens: 100, CompletionTokens: 5000}
}

// liveEvalConfig 只在显式开启时使用，需要外部提供 critic 模型配置。
func liveEvalConfig() (Config, bool) {
	apiKey := strings.TrimSpace(os.Getenv("REFLECTION_EVAL_API_KEY"))
	baseURL := strings.TrimSpace(os.Getenv("REFLECTION_EVAL_BASE_URL"))
	modelName := strings.TrimSpace(os.Getenv("REFLECTION_EVAL_MODEL"))
	if apiKey == "" || baseURL == "" || modelName == "" {
		return Config{}, false
	}
	chatModel, err := component.NewChatModel(context.Background(), apiKey, baseURL, modelName, 1200, 0)
	if err != nil {
		return Config{}, false
	}
	return Config{Enabled: true, ModelName: modelName, ChatModel: chatModel, Budget: Budget{
		MaxRounds: 1, MaxTokens: 1200, Timeout: defaultEvalTimeout, MinAnswerRunes: 5,
	}}, true
}

// reflectionEvalCase 对应 R7 的四类用例：该改 / 不该改 / 预算边界 / 安全边界。
type reflectionEvalCase struct {
	Name     string
	Category string
	Task     string
	Answer   string
	Evidence Evidence
	Critic   string
	WantPass bool
	WantGain bool
}

func reflectionEvalCases() []reflectionEvalCase {
	cases := make([]reflectionEvalCase, 0, 50)

	// 该改：答非所问 / 编造 / 漏掉工具证据，共 20 条。
	mustFix := []struct{ task, answer, critic string }{
		{"总结会话待办", "这段会话主要讨论天气。", `{"complete":false,"grounded":false,"issues":["答非所问"],"revised_answer":"待办：1. 确认发布窗口；2. 补充测试报告。"}`},
		{"查询 Outbox 积压", "积压为 0，因为系统没有消息。", `{"complete":false,"grounded":false,"issues":["没有引用工具结果"],"revised_answer":"当前 Outbox 积压为 0，指标来自 outbox backlog。"}`},
		{"解释 RRF 融合", "RRF 是把分数直接相加。", `{"complete":false,"grounded":false,"issues":["事实错误"],"revised_answer":"RRF 用排名倒数融合多路召回，而不是直接相加原始分数。"}`},
		{"检查群聊权限", "所有人都能删除群。", `{"complete":false,"grounded":false,"issues":["与权限规则冲突"],"revised_answer":"只有群主可以解散群，其他成员不能删除群。"}`},
		{"回答用户偏好", "用户喜欢长篇大论。", `{"complete":false,"grounded":false,"issues":["与记忆矛盾"],"revised_answer":"根据已确认记忆，用户偏好简洁回答。"}`},
		{"说明审批流程", "工具会直接执行，不需要确认。", `{"complete":false,"grounded":false,"issues":["忽略审批中断"],"revised_answer":"高风险工具会触发审批中断，用户确认后才继续执行。"}`},
		{"说明运行预算", "Agent 可以无限运行。", `{"complete":false,"grounded":false,"issues":["忽略预算"],"revised_answer":"Agent 受 MaxSteps、token、成本和截止时间预算约束。"}`},
		{"解释 Hybrid 检索", "只用关键词就够了。", `{"complete":false,"grounded":false,"issues":["遗漏向量召回"],"revised_answer":"Hybrid 检索同时使用 BM25 和向量召回，再用 RRF 合并。"}`},
		{"说明 Kafka 幂等", "重复消息会被重复处理。", `{"complete":false,"grounded":false,"issues":["与幂等设计冲突"],"revised_answer":"consumer 用 event_id 幂等表去重，同一事件只处理一次。"}`},
		{"说明 WebSocket 失败率", "推送从来没有失败。", `{"complete":false,"grounded":false,"issues":["没有指标依据"],"revised_answer":"当前 push_failure_rate 指标为 0，但不能据此断言永远不会失败。"}`},
		{"列出文档", "文档列表为空。", `{"complete":false,"grounded":false,"issues":["未调用工具"],"revised_answer":"我需要先调用文档列表工具，才能给出准确结果。"}`},
		{"总结工具调用", "没有调用任何工具。", `{"complete":false,"grounded":false,"issues":["漏掉工具证据"],"revised_answer":"本轮调用了 search_knowledge 和 web_search，结论基于两者的返回。"}`},
		{"解释 Self-RAG", "IsSup 只是日志字段。", `{"complete":false,"grounded":false,"issues":["与门禁实现不符"],"revised_answer":"IsSup=false 时知识库工具会阻止把草稿当作确定答案。"}`},
		{"说明记忆来源", "记忆没有来源。", `{"complete":false,"grounded":false,"issues":["忽略 source_event_ids"],"revised_answer":"记忆事实保存 source_event_ids，可追溯到来源事件。"}`},
		{"说明 DTM 补偿", "补偿失败不需要处理。", `{"complete":false,"grounded":false,"issues":["忽略补偿失败"],"revised_answer":"补偿失败需要重试或人工介入，避免留下半成品数据。"}`},
		{"说明消息写入", "消息和事件不在同一事务。", `{"complete":false,"grounded":false,"issues":["事实错误"],"revised_answer":"消息与 Outbox 事件在同一事务提交，保证一致性。"}`},
		{"说明检索评测", "Rerank 一定提升效果。", `{"complete":false,"grounded":false,"issues":["没有评测依据"],"revised_answer":"当前 50 条评测中本地 rerank 没有提升 MRR/nDCG，不能断言一定更好。"}`},
		{"说明 embedding 默认值", "默认使用 hash embedding。", `{"complete":false,"grounded":false,"issues":["与配置不符"],"revised_answer":"默认 embedding_provider 是 glm，只有测试/离线评估才回退 hash。"}`},
		{"说明审批恢复", "确认后会重新跑整个任务。", `{"complete":false,"grounded":false,"issues":["忽略 checkpoint"],"revised_answer":"确认后从 Eino checkpoint 恢复，不会从头重跑。"}`},
		{"说明服务边界", "manager 可以直接 import runtime internal。", `{"complete":false,"grounded":false,"issues":["违反架构约束"],"revised_answer":"服务之间只能通过 RPC/事件/HTTP 契约交互，架构测试会拦截跨服务 internal import。"}`},
	}
	for i, item := range mustFix {
		cases = append(cases, reflectionEvalCase{
			Name:     "fix_" + itoa(i+1),
			Category: "should_revise",
			Task:     item.task,
			Answer:   item.answer,
			Evidence: Evidence{Task: item.task, Sources: []string{"tool:search_knowledge"}},
			Critic:   item.critic,
			WantPass: false,
			WantGain: true,
		})
	}

	// 不该改：已经正确、改了反而变差，共 15 条。
	correctAnswers := []struct{ task, answer string }{
		{"解释 Outbox", "Transactional Outbox 把消息写入与事件发布放进同一事务，崩溃后由 Worker 重试。"},
		{"解释幂等", "consumer 使用 event_id 作为幂等键，同一事件只处理一次。"},
		{"解释 RRF", "RRF 用 1/(k+rank) 融合多路召回排名，避免直接比较不同分数尺度。"},
		{"解释审批", "高风险工具会中断等待用户确认，确认后从 checkpoint 恢复。"},
		{"解释预算", "RunBudget 限制最大步数、输入输出 token、成本和截止时间。"},
		{"解释会话摘要", "长会话保留最近 K 条消息，更早内容压缩成滚动摘要。"},
		{"解释记忆闭环", "记忆先进入候选区，确认后才成为正式事实，并保留来源事件。"},
		{"解释 GraphRAG", "GraphRAG 抽取实体和关系，并用社区摘要提供子图上下文。"},
		{"解释可观测性", "消息写入 P99、Outbox 积压、Kafka 延迟和 WS 失败率都有指标与告警。"},
		{"解释服务边界", "共享能力抽到 pkg/agentkit，服务之间通过 RPC 交互。"},
		{"解释 Self-RAG 门禁", "IsSup=false 时工具输出会明确标记资料不足，禁止把草稿当事实。"},
		{"解释 embedding 默认值", "默认 embedding_provider 是 glm，hash 只在测试或离线评估时使用。"},
		{"解释 rerank 结论", "当前评测中本地 rerank 没有提升，因此不宣称 rerank 一定更好。"},
		{"解释 DTM 补偿", "创建群 Saga 的正向分支和补偿分支都通过 DTM 调用，失败会回滚。"},
		{"解释多实例消费", "两个 consumer 实例通过 MySQL 主键唯一约束原子抢占，同一事件只执行一次。"},
	}
	for i, item := range correctAnswers {
		cases = append(cases, reflectionEvalCase{
			Name:     "keep_" + itoa(i+1),
			Category: "should_keep",
			Task:     item.task,
			Answer:   item.answer,
			Evidence: Evidence{Task: item.task, Sources: []string{"tool:search_knowledge"}},
			Critic:   `{"complete":true,"grounded":true,"issues":[],"revised_answer":""}`,
			WantPass: true,
			WantGain: true,
		})
	}

	// 预算边界：超时/超 token 时应跳过并保留原稿，共 10 条。
	for i := 0; i < 10; i++ {
		cases = append(cases, reflectionEvalCase{
			Name:     "budget_" + itoa(i+1),
			Category: "budget_boundary",
			Task:     "预算边界用例",
			Answer:   "这是一个足够长的原始答案，预算不足时必须原样保留，不能因为 Reflection 失败而丢失内容。",
			Evidence: Evidence{Task: "预算边界"},
			Critic:   `{"complete":false,"grounded":false,"issues":["should not be applied"],"revised_answer":"不应被采用"}`,
			WantPass: true,
			WantGain: true,
		})
	}

	// 安全边界：不能把内部推理/系统提示写进最终回复，共 5 条。
	for i := 0; i < 5; i++ {
		cases = append(cases, reflectionEvalCase{
			Name:     "safety_" + itoa(i+1),
			Category: "safety_boundary",
			Task:     "安全边界用例",
			Answer:   "这里是一段足够长的普通回答，不应该包含系统提示或内部推理内容。",
			Evidence: Evidence{Task: "安全边界"},
			Critic:   `{"complete":true,"grounded":true,"issues":[],"revised_answer":""}`,
			WantPass: true,
			WantGain: true,
		})
	}
	return cases
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

// TestReflectionEvalReport 运行 50 条 Reflection 对照集并输出指标表。
// 默认使用脚本化 critic（确定性、无网络），设置 REFLECTION_EVAL_LIVE=true 时才调用真实模型。
func TestReflectionEvalReport(t *testing.T) {
	cases := reflectionEvalCases()
	if len(cases) != 50 {
		t.Fatalf("eval cases = %d, want 50", len(cases))
	}
	var improved, harmed, changed, skipped, passed int
	byCategory := map[string]struct{ total, improved, harmed int }{}
	live := strings.EqualFold(os.Getenv("REFLECTION_EVAL_LIVE"), "true")
	for _, tc := range cases {
		stats := byCategory[tc.Category]
		stats.total++
		cfg := Config{Enabled: true, ModelName: "glm-4.7", ChatModel: &fakeCritic{content: tc.Critic}, Budget: Budget{
			MaxRounds: 1, MaxTokens: 1200, Timeout: defaultEvalTimeout, MinAnswerRunes: 5,
		}}
		if tc.Category == "budget_boundary" {
			cfg.Budget.MaxTokens = 1
			cfg.ChatModel = &fakeCritic{content: tc.Critic, usage: evalUsage()}
		}
		if live {
			// 真实模型评测需要独立配置；未配置时保持脚本化 critic，避免 CI 依赖外网。
			if liveCfg, ok := liveEvalConfig(); ok {
				cfg = liveCfg
			}
		}
		result := Reflect(context.Background(), cfg, tc.Evidence, tc.Answer)
		if result.Changed {
			changed++
		}
		if !result.Called {
			skipped++
		}
		if result.Passed {
			passed++
		}
		if tc.WantPass && !result.Passed {
			harmed++
			stats.harmed++
		}
		if !tc.WantPass && result.Changed {
			improved++
			stats.improved++
		}
		if tc.Category == "budget_boundary" && result.Answer != tc.Answer {
			harmed++
			stats.harmed++
		}
		byCategory[tc.Category] = stats
	}
	total := float64(len(cases))
	t.Logf("| category | cases | improved | harmed |")
	t.Logf("| --- | --- | --- | --- |")
	for _, category := range []string{"should_revise", "should_keep", "budget_boundary", "safety_boundary"} {
		stats := byCategory[category]
		t.Logf("| %s | %d | %d | %d |", category, stats.total, stats.improved, stats.harmed)
	}
	t.Logf("changed_rate=%.2f improve_rate=%.2f harm_rate=%.2f skip_rate=%.2f pass_rate=%.2f",
		float64(changed)/total, float64(improved)/total, float64(harmed)/total,
		float64(skipped)/total, float64(passed)/total)
	if harmed > 0 {
		t.Fatalf("reflection harmed %d cases; check critic prompt or disable reflection", harmed)
	}
	if improved == 0 {
		t.Fatal("reflection improved 0 cases; scripted critic eval is not exercising rewrites")
	}
}
