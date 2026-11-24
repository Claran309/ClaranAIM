package service

import (
	"ClaranAIM/internal/agent-runtime-service/reflection"
	"ClaranAIM/internal/agent-runtime-service/store"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/pkg/observability"
	"context"
	"encoding/json"
	"log"
	"time"
)

// runReflection 在 RunAgent 成功后执行一次 critic 自检。
func (s *runtimeServiceImpl) runReflection(ctx context.Context, req *bot_runtime.RunAgentReq, run *store.RunInfo, answer string) reflection.Result {
	cfg := s.reflectionConfig(req.Bot)
	evidence := reflection.Evidence{
		Task:        req.Input,
		HasHighRisk: false,
	}
	result := reflection.Reflect(ctx, cfg, evidence, answer)
	if result.Called {
		log.Printf("Agent Reflection: run=%s %s", run.RunID, result.Summary())
	}
	recordReflectionMetrics(result)
	return result
}

// runReflectionForResume 在审批恢复后执行一次 critic 自检。
// 审批场景默认视为高风险，避免“工具刚执行完就宣称完成”。
func (s *runtimeServiceImpl) runReflectionForResume(ctx context.Context, req *bot_runtime.ResumeAgentReq, run *store.RunInfo, answer string) reflection.Result {
	cfg := s.reflectionConfig(req.Bot)
	evidence := reflection.Evidence{
		Task:        "恢复被审批中断的 Agent 运行并完成用户任务",
		HasHighRisk: true,
		ToolCalls:   []string{run.InterruptedTool},
	}
	result := reflection.Reflect(ctx, cfg, evidence, answer)
	if result.Called {
		log.Printf("Agent Reflection(resume): run=%s %s", run.RunID, result.Summary())
	}
	recordReflectionMetrics(result)
	return result
}

func (s *runtimeServiceImpl) reflectionConfig(bot *bot_runtime.RuntimeBotConfig) reflection.Config {
	cfg := s.reflection
	if bot == nil {
		return cfg
	}
	if cfg.ModelName == "" {
		cfg.ModelName = bot.ModelName
	}
	return cfg
}

// attachReflection 把 Reflection 结果写入运行记录，不包含答案正文。
func attachReflection(run *store.RunInfo, result reflection.Result) {
	if run == nil {
		return
	}
	payload, err := json.Marshal(map[string]interface{}{
		"called":         result.Called,
		"passed":         result.Passed,
		"changed":        result.Changed,
		"rounds":         result.Rounds,
		"issues":         result.Issues,
		"skipped_reason": result.SkippedReason,
		"input_tokens":   result.InputTokens,
		"output_tokens":  result.OutputTokens,
		"cost_usd":       result.CostUSD,
		"latency_ms":     result.LatencyMS,
		"updated_at":     time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return
	}
	run.ReflectionJSON = string(payload)
}

// recordReflectionMetrics 把 Reflection 结果写入 Prometheus/OTel，便于按结果聚合失败率。
func recordReflectionMetrics(result reflection.Result) {
	var status string
	switch {
	case !result.Called:
		status = "skip"
	case result.Passed:
		status = "pass"
	case result.Changed:
		status = "revise"
	default:
		status = "fail"
	}
	observability.RecordReflection(status, time.Duration(result.LatencyMS)*time.Millisecond,
		result.InputTokens, result.OutputTokens, result.Changed, len(result.Issues))
}
