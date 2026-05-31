// Package handler 实现 agent-manager-service 的 Kitex RPC 入口。
package handler

import (
	"ClaranAIM/internal/agent-manager-service/model"
	"ClaranAIM/internal/agent-manager-service/service"
	"ClaranAIM/kitex_gen/bot"
	"ClaranAIM/pkg/config"
	"context"
)

// agentServiceImpl 是 agent-manager-service 的 Kitex 服务端实现。
type agentServiceImpl struct {
	svc service.AgentService
	cfg *config.Config
}

// NewAgentServiceImpl 将业务服务和进程配置注入 Kitex RPC 实现。
func NewAgentServiceImpl(svc service.AgentService, cfg *config.Config) bot.BotService {
	return &agentServiceImpl{svc: svc, cfg: cfg}
}

// defaultLLM 返回平台默认模型配置，供 internal Agent 或未显式指定模型时兜底。
func (h *agentServiceImpl) defaultLLM() (apiKey, baseURL, model string) {
	return h.cfg.LLM.DefaultAPIKey, h.cfg.LLM.DefaultBaseURL, h.cfg.LLM.DefaultModel
}

// CreateBot 处理生成代码中的 Agent 创建 RPC，并注入默认 LLM 配置。
func (h *agentServiceImpl) CreateBot(ctx context.Context, req *bot.CreateBotReq) (resp *bot.CreateBotResp, err error) {
	apiKey, baseURL, model := h.defaultLLM()
	b, err := h.svc.CreateBot(ctx, req.Name, req.Type, req.Description, req.ModelName, req.ApiKey, req.BaseUrl, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.Avatar, req.Signature, req.WorkspaceRoot, req.ToolPolicy, req.OwnerId, req.ContextMessageLimit, req.MemoryRecallLimit, req.MaxOutputTokens, req.Temperature, req.GroupTriggerMode, req.AutoReplyEnabled, apiKey, baseURL, model)
	if err != nil {
		return &bot.CreateBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.CreateBotResp{Success: true, BotId: b.ID, Msg: "创建成功"}, nil
}

// UpdateBot 处理 Agent 更新 RPC，权限检查由 service 层完成。
func (h *agentServiceImpl) UpdateBot(ctx context.Context, req *bot.UpdateBotReq) (resp *bot.UpdateBotResp, err error) {
	apiKey, baseURL, model := h.defaultLLM()
	err = h.svc.UpdateBot(ctx, req.BotId, req.OperatorId, req.Name, req.Description, req.ModelName, req.ApiKey, req.BaseUrl, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.Avatar, req.Signature, req.WorkspaceRoot, req.ToolPolicy, req.IsActive, req.IsActiveSet, req.ContextMessageLimit, req.MemoryRecallLimit, req.MaxOutputTokens, req.Temperature, req.GroupTriggerMode, req.AutoReplyEnabled, apiKey, baseURL, model)
	if err != nil {
		return &bot.UpdateBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.UpdateBotResp{Success: true, Msg: "更新成功"}, nil
}

// GetBot 返回单个 Agent 配置。
//
// 当前 BotInfo IDL 不包含 API Key 或 has_api_key 字段，因此这里不能返回密钥状态；
// 前端如果需要展示“已配置密钥”，应先扩展 IDL 后再安全返回脱敏状态。
func (h *agentServiceImpl) GetBot(ctx context.Context, req *bot.GetBotReq) (resp *bot.GetBotResp, err error) {
	b, err := h.svc.GetBot(ctx, req.BotId)
	if err != nil {
		return &bot.GetBotResp{Success: false, Msg: err.Error()}, nil
	}
	if b == nil {
		return &bot.GetBotResp{Success: false, Msg: "Agent不存在"}, nil
	}
	return &bot.GetBotResp{
		Success: true,
		Bot:     botConfigFromModel(b),
	}, nil
}

// botConfigFromModel 将数据库模型转换为 Thrift 返回结构。
func botConfigFromModel(b *model.Bot) *bot.BotInfo {
	return &bot.BotInfo{
		Id:                  b.ID,
		Name:                b.Name,
		Type:                b.Type,
		Description:         b.Description,
		ModelName:           b.ModelName,
		BaseUrl:             b.BaseURL,
		SystemPrompt:        b.SystemPrompt,
		SkillsDir:           b.SkillsDir,
		AgentRoot:           b.AgentRoot,
		IsActive:            b.IsActive,
		OwnerId:             b.OwnerID,
		CreatedAt:           b.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:           b.UpdatedAt.Format("2006-01-02 15:04:05"),
		AgentUserId:         b.AgentUserID,
		Avatar:              b.Avatar,
		Signature:           b.Signature,
		WorkspaceRoot:       b.WorkspaceRoot,
		ToolPolicy:          b.ToolPolicy,
		ContextMessageLimit: b.ContextMessageLimit,
		MemoryRecallLimit:   b.MemoryRecallLimit,
		MaxOutputTokens:     b.MaxOutputTokens,
		Temperature:         b.Temperature,
		GroupTriggerMode:    b.GroupTriggerMode,
		AutoReplyEnabled:    b.AutoReplyEnabled,
	}
}

// ListBots 返回 Agent 配置列表。
func (h *agentServiceImpl) ListBots(ctx context.Context, req *bot.ListBotsReq) (resp *bot.ListBotsResp, err error) {
	bots, err := h.svc.ListBots(ctx, req.OwnerId, req.Type)
	if err != nil {
		return &bot.ListBotsResp{Success: false, Msg: err.Error()}, nil
	}
	var botList []*bot.BotInfo
	for _, b := range bots {
		botList = append(botList, botConfigFromModel(&b))
	}
	return &bot.ListBotsResp{Success: true, Bots: botList}, nil
}

// DeleteBot 删除 Agent，所有权检查由 service 层完成。
func (h *agentServiceImpl) DeleteBot(ctx context.Context, req *bot.DeleteBotReq) (resp *bot.DeleteBotResp, err error) {
	err = h.svc.DeleteBot(ctx, req.BotId, req.OperatorId)
	if err != nil {
		return &bot.DeleteBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.DeleteBotResp{Success: true, Msg: "删除成功"}, nil
}

// ChatWithBot 执行一轮 Agent 对话并返回回复、token 和费用信息。
func (h *agentServiceImpl) ChatWithBot(ctx context.Context, req *bot.ChatWithBotReq) (resp *bot.ChatWithBotResp, err error) {
	result, err := h.svc.ChatWithBot(ctx, req.BotId, req.UserId, req.ConversationId, req.Message)
	if err != nil {
		return &bot.ChatWithBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.ChatWithBotResp{
		Success:      true,
		Reply:        result.Reply,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Cost:         result.Cost,
		Msg:          result.Status,
	}, nil
}

// SummarizeConversation 执行会话总结任务。
func (h *agentServiceImpl) SummarizeConversation(ctx context.Context, req *bot.AgentTaskReq) (*bot.AgentTaskResp, error) {
	return h.runTask(ctx, req, "summary")
}

// AskConversation 执行基于会话上下文的问答任务。
func (h *agentServiceImpl) AskConversation(ctx context.Context, req *bot.AgentTaskReq) (*bot.AgentTaskResp, error) {
	return h.runTask(ctx, req, "ask")
}

// ExtractInsights 执行会话洞察提取任务。
func (h *agentServiceImpl) ExtractInsights(ctx context.Context, req *bot.AgentTaskReq) (*bot.AgentTaskResp, error) {
	return h.runTask(ctx, req, "insights")
}

// GenerateReplyCandidates 生成候选回复。
func (h *agentServiceImpl) GenerateReplyCandidates(ctx context.Context, req *bot.AgentTaskReq) (*bot.AgentTaskResp, error) {
	return h.runTask(ctx, req, "reply_candidates")
}

// runTask 统一转发结构化 Agent 任务到 service 层。
func (h *agentServiceImpl) runTask(ctx context.Context, req *bot.AgentTaskReq, taskType string) (*bot.AgentTaskResp, error) {
	result, err := h.svc.RunAgentTask(ctx, req.BotId, req.UserId, req.ConversationId, taskType, req.Question)
	if err != nil {
		return &bot.AgentTaskResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.AgentTaskResp{Success: true, Result_: result, Msg: "ok"}, nil
}

// GrantPermission 授予 Agent 协作权限。
func (h *agentServiceImpl) GrantPermission(ctx context.Context, req *bot.GrantPermissionReq) (*bot.GrantPermissionResp, error) {
	if err := h.svc.GrantPermission(ctx, req.BotId, req.OperatorId, req.UserId, req.Role); err != nil {
		return &bot.GrantPermissionResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.GrantPermissionResp{Success: true, Msg: "授权成功"}, nil
}

// RevokePermission 撤销 Agent 协作权限。
func (h *agentServiceImpl) RevokePermission(ctx context.Context, req *bot.RevokePermissionReq) (*bot.RevokePermissionResp, error) {
	if err := h.svc.RevokePermission(ctx, req.BotId, req.OperatorId, req.UserId); err != nil {
		return &bot.RevokePermissionResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.RevokePermissionResp{Success: true, Msg: "撤销成功"}, nil
}

// ListPermissions 查询 Agent 协作权限列表。
func (h *agentServiceImpl) ListPermissions(ctx context.Context, req *bot.ListPermissionsReq) (*bot.ListPermissionsResp, error) {
	permissions, err := h.svc.ListPermissions(ctx, req.BotId, req.OperatorId)
	if err != nil {
		return &bot.ListPermissionsResp{Success: false, Msg: err.Error()}, nil
	}
	list := make([]*bot.BotPermission, 0, len(permissions))
	for _, p := range permissions {
		list = append(list, &bot.BotPermission{
			Id:        p.ID,
			BotId:     p.BotID,
			UserId:    p.UserID,
			Role:      p.Role,
			CreatedAt: p.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: p.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &bot.ListPermissionsResp{Success: true, Permissions: list}, nil
}

// CreateRoute 创建 Agent 路由规则。
func (h *agentServiceImpl) CreateRoute(ctx context.Context, req *bot.CreateRouteReq) (resp *bot.CreateRouteResp, err error) {
	route, err := h.svc.CreateRoute(ctx, req.BotId, req.RoutePattern, req.RouteType, req.Priority)
	if err != nil {
		return &bot.CreateRouteResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.CreateRouteResp{Success: true, RouteId: route.ID, Msg: "创建成功"}, nil
}

// ListRoutes 查询某个 Agent 的路由规则。
func (h *agentServiceImpl) ListRoutes(ctx context.Context, req *bot.ListRoutesReq) (resp *bot.ListRoutesResp, err error) {
	routes, err := h.svc.ListRoutes(ctx, req.BotId)
	if err != nil {
		return &bot.ListRoutesResp{Success: false, Msg: err.Error()}, nil
	}
	var routeList []*bot.BotRoute
	for _, r := range routes {
		routeList = append(routeList, &bot.BotRoute{
			Id:           r.ID,
			BotId:        r.BotID,
			RoutePattern: r.RoutePattern,
			RouteType:    r.RouteType,
			Priority:     r.Priority,
			CreatedAt:    r.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &bot.ListRoutesResp{Success: true, Routes: routeList}, nil
}

// DeleteRoute 删除一条路由规则。
func (h *agentServiceImpl) DeleteRoute(ctx context.Context, req *bot.DeleteRouteReq) (resp *bot.DeleteRouteResp, err error) {
	err = h.svc.DeleteRoute(ctx, req.RouteId, req.OperatorId)
	if err != nil {
		return &bot.DeleteRouteResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.DeleteRouteResp{Success: true, Msg: "删除成功"}, nil
}

// GetBilling 返回已记录的真实 token 和费用记录。
func (h *agentServiceImpl) GetBilling(ctx context.Context, req *bot.GetBillingReq) (resp *bot.GetBillingResp, err error) {
	records, total, err := h.svc.GetBilling(ctx, req.BotId, req.UserId, req.Limit, req.Offset)
	if err != nil {
		return &bot.GetBillingResp{Success: false, Msg: err.Error()}, nil
	}
	var billingList []*bot.BillingRecord
	for _, r := range records {
		billingList = append(billingList, &bot.BillingRecord{
			Id:             r.ID,
			BotId:          r.BotID,
			UserId:         r.UserID,
			ConversationId: r.ConversationID,
			InputTokens:    r.InputTokens,
			OutputTokens:   r.OutputTokens,
			Cost:           r.Cost,
			ModelName:      r.ModelName,
			CreatedAt:      r.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &bot.GetBillingResp{Success: true, Records: billingList, Total: total}, nil
}
