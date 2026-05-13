package handler

import (
	"ClaranAIM/internal/bot-manager-service/service"
	"ClaranAIM/kitex_gen/bot"
	"ClaranAIM/pkg/config"
	"context"
)

type BotServiceImpl struct {
	svc service.BotService
	cfg *config.Config
}

func NewBotServiceImpl(svc service.BotService, cfg *config.Config) bot.BotService {
	return &BotServiceImpl{svc: svc, cfg: cfg}
}

func (h *BotServiceImpl) defaultLLM() (apiKey, baseURL, model string) {
	return h.cfg.LLM.DefaultAPIKey, h.cfg.LLM.DefaultBaseURL, h.cfg.LLM.DefaultModel
}

func (h *BotServiceImpl) CreateBot(ctx context.Context, req *bot.CreateBotReq) (resp *bot.CreateBotResp, err error) {
	apiKey, baseURL, model := h.defaultLLM()
	b, err := h.svc.CreateBot(ctx, req.Name, req.Type, req.Description, req.ModelName, req.ApiKey, req.BaseUrl, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.OwnerId, apiKey, baseURL, model)
	if err != nil {
		return &bot.CreateBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.CreateBotResp{Success: true, BotId: b.ID, Msg: "创建成功"}, nil
}

func (h *BotServiceImpl) UpdateBot(ctx context.Context, req *bot.UpdateBotReq) (resp *bot.UpdateBotResp, err error) {
	apiKey, baseURL, model := h.defaultLLM()
	err = h.svc.UpdateBot(ctx, req.BotId, req.OperatorId, req.Name, req.Description, req.ModelName, req.ApiKey, req.BaseUrl, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.IsActive, apiKey, baseURL, model)
	if err != nil {
		return &bot.UpdateBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.UpdateBotResp{Success: true, Msg: "更新成功"}, nil
}

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
		Bot: &bot.BotConfig{
			Id:           b.ID,
			Name:         b.Name,
			Type:         b.Type,
			Description:  b.Description,
			ModelName:    b.ModelName,
			ApiKey:       apiKey,
			BaseUrl:      b.BaseURL,
			SystemPrompt: b.SystemPrompt,
			SkillsDir:    b.SkillsDir,
			AgentRoot:    b.AgentRoot,
			IsActive:     b.IsActive,
			OwnerId:      b.OwnerID,
			CreatedAt:    b.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:    b.UpdatedAt.Format("2006-01-02 15:04:05"),
		},
	}, nil
}

func (h *BotServiceImpl) ListBots(ctx context.Context, req *bot.ListBotsReq) (resp *bot.ListBotsResp, err error) {
	bots, err := h.svc.ListBots(ctx, req.OwnerId, req.Type)
	if err != nil {
		return &bot.ListBotsResp{Success: false, Msg: err.Error()}, nil
	}
	var botList []*bot.BotConfig
	for _, b := range bots {
		apiKey := b.APIKey
		if b.Type == "internal" {
			apiKey = "***"
		}
		botList = append(botList, &bot.BotConfig{
			Id:           b.ID,
			Name:         b.Name,
			Type:         b.Type,
			Description:  b.Description,
			ModelName:    b.ModelName,
			ApiKey:       apiKey,
			BaseUrl:      b.BaseURL,
			SystemPrompt: b.SystemPrompt,
			SkillsDir:    b.SkillsDir,
			AgentRoot:    b.AgentRoot,
			IsActive:     b.IsActive,
			OwnerId:      b.OwnerID,
			CreatedAt:    b.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:    b.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &bot.ListBotsResp{Success: true, Bots: botList}, nil
}

func (h *BotServiceImpl) DeleteBot(ctx context.Context, req *bot.DeleteBotReq) (resp *bot.DeleteBotResp, err error) {
	err = h.svc.DeleteBot(ctx, req.BotId, req.OperatorId)
	if err != nil {
		return &bot.DeleteBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.DeleteBotResp{Success: true, Msg: "删除成功"}, nil
}

func (h *BotServiceImpl) ChatWithBot(ctx context.Context, req *bot.ChatWithBotReq) (resp *bot.ChatWithBotResp, err error) {
	reply, _, err := h.svc.ChatWithBot(ctx, req.BotId, req.UserId, req.ConversationId, req.Message)
	if err != nil {
		return &bot.ChatWithBotResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.ChatWithBotResp{Success: true, Reply: reply}, nil
}

func (h *BotServiceImpl) CreateRoute(ctx context.Context, req *bot.CreateRouteReq) (resp *bot.CreateRouteResp, err error) {
	route, err := h.svc.CreateRoute(ctx, req.BotId, req.RoutePattern, req.RouteType, req.Priority)
	if err != nil {
		return &bot.CreateRouteResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.CreateRouteResp{Success: true, RouteId: route.ID, Msg: "创建成功"}, nil
}

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

func (h *BotServiceImpl) DeleteRoute(ctx context.Context, req *bot.DeleteRouteReq) (resp *bot.DeleteRouteResp, err error) {
	err = h.svc.DeleteRoute(ctx, req.RouteId, req.OperatorId)
	if err != nil {
		return &bot.DeleteRouteResp{Success: false, Msg: err.Error()}, nil
	}
	return &bot.DeleteRouteResp{Success: true, Msg: "删除成功"}, nil
}

func (h *BotServiceImpl) GetBilling(ctx context.Context, req *bot.GetBillingReq) (resp *bot.GetBillingResp, err error) {
	records, total, err := h.svc.GetBilling(ctx, req.BotId, req.UserId, req.Limit, req.Offset)
	if err != nil {
		return &bot.GetBillingResp{Success: false, Msg: err.Error()}, nil
	}
	var billingList []*bot.BillingRecord
	for _, r := range records {
		billingList = append(billingList, &bot.BillingRecord{
			Id:        r.ID,
			BotId:     r.BotID,
			UserId:    r.UserID,
			Cost:      r.Cost,
			CreatedAt: r.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &bot.GetBillingResp{Success: true, Records: billingList, Total: total}, nil
}
