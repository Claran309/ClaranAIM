// Package handler implements bot-runtime-service Kitex handlers.
package handler

import (
	"ClaranAIM/internal/bot-runtime-service/service"
	"ClaranAIM/kitex_gen/bot_runtime"
	"context"
)

// BotRuntimeServiceImpl adapts runtime business logic to generated Kitex RPC.
type BotRuntimeServiceImpl struct {
	svc service.BotRuntimeService
}

// NewBotRuntimeServiceImpl creates a runtime RPC handler.
func NewBotRuntimeServiceImpl(svc service.BotRuntimeService) bot_runtime.BotRuntimeService {
	return &BotRuntimeServiceImpl{svc: svc}
}

func (h *BotRuntimeServiceImpl) RunAgent(ctx context.Context, req *bot_runtime.RunAgentReq) (*bot_runtime.RunAgentResp, error) {
	return h.svc.RunAgent(ctx, req)
}

func (h *BotRuntimeServiceImpl) SummarizeConversation(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "summary"
	return h.svc.RunTask(ctx, req)
}

func (h *BotRuntimeServiceImpl) AskConversation(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "ask"
	return h.svc.RunTask(ctx, req)
}

func (h *BotRuntimeServiceImpl) ExtractInsights(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "insights"
	return h.svc.RunTask(ctx, req)
}

func (h *BotRuntimeServiceImpl) GenerateReplyCandidates(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	req.TaskType = "reply_candidates"
	return h.svc.RunTask(ctx, req)
}

func (h *BotRuntimeServiceImpl) GetAgentSessions(ctx context.Context, req *bot_runtime.GetAgentSessionReq) (*bot_runtime.GetAgentSessionResp, error) {
	return h.svc.GetAgentSessions(ctx, req)
}
