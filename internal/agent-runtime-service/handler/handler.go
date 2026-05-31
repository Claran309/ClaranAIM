// Package handler 实现 agent-runtime-service 的 Kitex RPC 适配层。
package handler

import (
	"ClaranAIM/internal/agent-runtime-service/service"
	"ClaranAIM/kitex_gen/bot_runtime"
	"context"
)

// AgentRuntimeServiceImpl 将 runtime 业务服务适配为 Kitex 生成的 RPC 接口。
type AgentRuntimeServiceImpl struct {
	svc service.AgentRuntimeService
}

// NewAgentRuntimeServiceImpl 创建 runtime RPC handler。
func NewAgentRuntimeServiceImpl(svc service.AgentRuntimeService) bot_runtime.BotRuntimeService {
	return &AgentRuntimeServiceImpl{svc: svc}
}

// RunAgent 处理通用 Agent 执行请求，保持调用方传入的 TaskType 和 SessionKey 不变。
func (h *AgentRuntimeServiceImpl) RunAgent(ctx context.Context, req *bot_runtime.RunAgentReq) (*bot_runtime.RunAgentResp, error) {
	return h.svc.RunAgent(ctx, req)
}

// SummarizeConversation 将 RPC 方法名映射为 summary 任务，具体上下文构建和模型调用交给 service 层。
func (h *AgentRuntimeServiceImpl) SummarizeConversation(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "summary"
	return h.svc.RunTask(ctx, req)
}

// AskConversation 将“基于会话提问”入口标记为 ask 任务，便于 runtime 选择问答型提示词。
func (h *AgentRuntimeServiceImpl) AskConversation(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "ask"
	return h.svc.RunTask(ctx, req)
}

// ExtractInsights 标记洞察提取任务，输出由 service 层约束为结论、分歧、风险、待办等结构。
func (h *AgentRuntimeServiceImpl) ExtractInsights(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "insights"
	return h.svc.RunTask(ctx, req)
}

// GenerateReplyCandidates 标记候选回复任务，供前端展示多条可选回复而不是直接发送消息。
func (h *AgentRuntimeServiceImpl) GenerateReplyCandidates(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "reply_candidates"
	return h.svc.RunTask(ctx, req)
}

// GetAgentSessions 读取 runtime 持久化的 JSONL 会话索引，用于前端恢复连续对话历史。
func (h *AgentRuntimeServiceImpl) GetAgentSessions(ctx context.Context, req *bot_runtime.GetAgentSessionReq) (*bot_runtime.GetAgentSessionResp, error) {
	return h.svc.GetAgentSessions(ctx, req)
}
