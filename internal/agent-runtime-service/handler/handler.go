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

// RunAgent 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (h *AgentRuntimeServiceImpl) RunAgent(ctx context.Context, req *bot_runtime.RunAgentReq) (*bot_runtime.RunAgentResp, error) {
	return h.svc.RunAgent(ctx, req)
}

// SummarizeConversation 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (h *AgentRuntimeServiceImpl) SummarizeConversation(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "summary"
	return h.svc.RunTask(ctx, req)
}

// AskConversation 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (h *AgentRuntimeServiceImpl) AskConversation(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "ask"
	return h.svc.RunTask(ctx, req)
}

// ExtractInsights 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (h *AgentRuntimeServiceImpl) ExtractInsights(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "insights"
	return h.svc.RunTask(ctx, req)
}

// GenerateReplyCandidates 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (h *AgentRuntimeServiceImpl) GenerateReplyCandidates(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "reply_candidates"
	return h.svc.RunTask(ctx, req)
}

// GetAgentSessions 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (h *AgentRuntimeServiceImpl) GetAgentSessions(ctx context.Context, req *bot_runtime.GetAgentSessionReq) (*bot_runtime.GetAgentSessionResp, error) {
	return h.svc.GetAgentSessions(ctx, req)
}
