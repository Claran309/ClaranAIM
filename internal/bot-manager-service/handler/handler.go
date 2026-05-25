// Package handler implements bot-manager-service Kitex RPC handlers.
package handler

import (
	"ClaranAIM/internal/bot-manager-service/model"
	"ClaranAIM/internal/bot-manager-service/service"
	"ClaranAIM/kitex_gen/bot"
	"ClaranAIM/pkg/config"
	"context"
)

// BotServiceImpl is the Kitex server implementation for bot-manager-service.
type BotServiceImpl struct {
	svc service.BotService
	cfg *config.Config
}

// NewBotServiceImpl wires the business service and process config into Kitex.
func NewBotServiceImpl(svc service.BotService, cfg *config.Config) bot.BotService {
	return &BotServiceImpl{svc: svc, cfg: cfg}
}

func (h *BotServiceImpl) defaultLLM() (apiKey, baseURL, model string) {
	return h.cfg.LLM.DefaultAPIKey, h.cfg.LLM.DefaultBaseURL, h.cfg.LLM.DefaultModel
}

// CreateBot handles RPC bot creation with default LLM config fallback.
func (h *BotServiceImpl) CreateBot(ctx context.Context, req *bot.CreateBotReq) (resp *bot.CreateBotResp, err error) {
	apiKey, baseURL, model := h.defaultLLM()
	b, err := h.svc.CreateBot(ctx, req.Name, req.Type, req.Description, req.ModelName, req.ApiKey, req.BaseUrl, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.Avatar, req.Signature, req.WorkspaceRoot, req.ToolPolicy, req.OwnerId, apiKey, baseURL, model)
	if err != nil {
		return &bot.CreateBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.CreateBotResp{Success: true, BotId: b.ID, Msg: "创建成功"}, nil
}

// UpdateBot handles RPC bot updates and service-level ownership checks.
func (h *BotServiceImpl) UpdateBot(ctx context.Context, req *bot.UpdateBotReq) (resp *bot.UpdateBotResp, err error) {
	apiKey, baseURL, model := h.defaultLLM()
	err = h.svc.UpdateBot(ctx, req.BotId, req.OperatorId, req.Name, req.Description, req.ModelName, req.ApiKey, req.BaseUrl, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.Avatar, req.Signature, req.WorkspaceRoot, req.ToolPolicy, req.IsActive, req.IsActiveSet, apiKey, baseURL, model)
	if err != nil {
		return &bot.UpdateBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.UpdateBotResp{Success: true, Msg: "更新成功"}, nil
}

// GetBot returns one bot config and masks internal bot API keys.
func (h *BotServiceImpl) GetBot(ctx context.Context, req *bot.GetBotReq) (resp *bot.GetBotResp, err error) {
	b, err := h.svc.GetBot(ctx, req.BotId)
	if err != nil {
		return &bot.GetBotResp{Success: false, Msg: err.Error()}, nil
	}
	if b == nil {
		return &bot.GetBotResp{Success: false, Msg: "bot不存在"}, nil
	}
	apiKey := b.APIKey
	if b.Type == "internal" {
		apiKey = "***"
	}
	return &bot.GetBotResp{
		Success: true,
		Bot:     botConfigFromModel(b, apiKey),
	}, nil
}

func botConfigFromModel(b *model.Bot, apiKey string) *bot.BotInfo {
	return &bot.BotInfo{
		Id:            b.ID,
		Name:          b.Name,
		Type:          b.Type,
		Description:   b.Description,
		ModelName:     b.ModelName,
		BaseUrl:       b.BaseURL,
		SystemPrompt:  b.SystemPrompt,
		SkillsDir:     b.SkillsDir,
		AgentRoot:     b.AgentRoot,
		IsActive:      b.IsActive,
		OwnerId:       b.OwnerID,
		CreatedAt:     b.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:     b.UpdatedAt.Format("2006-01-02 15:04:05"),
		AgentUserId:   b.AgentUserID,
		Avatar:        b.Avatar,
		Signature:     b.Signature,
		WorkspaceRoot: b.WorkspaceRoot,
		ToolPolicy:    b.ToolPolicy,
	}
}

// ListBots returns bot configs and masks internal bot API keys.
func (h *BotServiceImpl) ListBots(ctx context.Context, req *bot.ListBotsReq) (resp *bot.ListBotsResp, err error) {
	bots, err := h.svc.ListBots(ctx, req.OwnerId, req.Type)
	if err != nil {
		return &bot.ListBotsResp{Success: false, Msg: err.Error()}, nil
	}
	var botList []*bot.BotInfo
	for _, b := range bots {
		apiKey := b.APIKey
		if b.Type == "internal" {
			apiKey = "***"
		}
		botList = append(botList, botConfigFromModel(&b, apiKey))
	}
	return &bot.ListBotsResp{Success: true, Bots: botList}, nil
}

// DeleteBot deletes a bot after service-level ownership checks.
func (h *BotServiceImpl) DeleteBot(ctx context.Context, req *bot.DeleteBotReq) (resp *bot.DeleteBotResp, err error) {
	err = h.svc.DeleteBot(ctx, req.BotId, req.OperatorId)
	if err != nil {
		return &bot.DeleteBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.DeleteBotResp{Success: true, Msg: "删除成功"}, nil
}

// ChatWithBot runs one bot chat turn and returns the assistant reply.
func (h *BotServiceImpl) ChatWithBot(ctx context.Context, req *bot.ChatWithBotReq) (resp *bot.ChatWithBotResp, err error) {
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

func (h *BotServiceImpl) SummarizeConversation(ctx context.Context, req *bot.AgentTaskReq) (*bot.AgentTaskResp, error) {
	return h.runTask(ctx, req, "summary")
}

func (h *BotServiceImpl) AskConversation(ctx context.Context, req *bot.AgentTaskReq) (*bot.AgentTaskResp, error) {
	return h.runTask(ctx, req, "ask")
}

func (h *BotServiceImpl) ExtractInsights(ctx context.Context, req *bot.AgentTaskReq) (*bot.AgentTaskResp, error) {
	return h.runTask(ctx, req, "insights")
}

func (h *BotServiceImpl) GenerateReplyCandidates(ctx context.Context, req *bot.AgentTaskReq) (*bot.AgentTaskResp, error) {
	return h.runTask(ctx, req, "reply_candidates")
}

func (h *BotServiceImpl) runTask(ctx context.Context, req *bot.AgentTaskReq, taskType string) (*bot.AgentTaskResp, error) {
	result, err := h.svc.RunAgentTask(ctx, req.BotId, req.UserId, req.ConversationId, taskType, req.Question)
	if err != nil {
		return &bot.AgentTaskResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.AgentTaskResp{Success: true, Result_: result, Msg: "ok"}, nil
}

func (h *BotServiceImpl) GrantPermission(ctx context.Context, req *bot.GrantPermissionReq) (*bot.GrantPermissionResp, error) {
	if err := h.svc.GrantPermission(ctx, req.BotId, req.OperatorId, req.UserId, req.Role); err != nil {
		return &bot.GrantPermissionResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.GrantPermissionResp{Success: true, Msg: "授权成功"}, nil
}

func (h *BotServiceImpl) RevokePermission(ctx context.Context, req *bot.RevokePermissionReq) (*bot.RevokePermissionResp, error) {
	if err := h.svc.RevokePermission(ctx, req.BotId, req.OperatorId, req.UserId); err != nil {
		return &bot.RevokePermissionResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.RevokePermissionResp{Success: true, Msg: "撤销成功"}, nil
}

func (h *BotServiceImpl) ListPermissions(ctx context.Context, req *bot.ListPermissionsReq) (*bot.ListPermissionsResp, error) {
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

// CreateRoute creates a bot routing rule.
func (h *BotServiceImpl) CreateRoute(ctx context.Context, req *bot.CreateRouteReq) (resp *bot.CreateRouteResp, err error) {
	route, err := h.svc.CreateRoute(ctx, req.BotId, req.RoutePattern, req.RouteType, req.Priority)
	if err != nil {
		return &bot.CreateRouteResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.CreateRouteResp{Success: true, RouteId: route.ID, Msg: "创建成功"}, nil
}

// ListRoutes returns routing rules for a bot.
func (h *BotServiceImpl) ListRoutes(ctx context.Context, req *bot.ListRoutesReq) (resp *bot.ListRoutesResp, err error) {
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

// DeleteRoute removes one routing rule.
func (h *BotServiceImpl) DeleteRoute(ctx context.Context, req *bot.DeleteRouteReq) (resp *bot.DeleteRouteResp, err error) {
	err = h.svc.DeleteRoute(ctx, req.RouteId, req.OperatorId)
	if err != nil {
		return &bot.DeleteRouteResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.DeleteRouteResp{Success: true, Msg: "删除成功"}, nil
}

// GetBilling returns measured token/cost records.
func (h *BotServiceImpl) GetBilling(ctx context.Context, req *bot.GetBillingReq) (resp *bot.GetBillingResp, err error) {
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
