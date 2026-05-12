package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/pkg/response"
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

type BotHandler struct{}

func NewBotHandler() *BotHandler {
	return &BotHandler{}
}

func (h *BotHandler) CreateBot(ctx context.Context, c *app.RequestContext) {
	type createBotReq struct {
		Name         string `json:"name"`
		Type         string `json:"type"`
		Description  string `json:"description"`
		ModelName    string `json:"model_name"`
		APIKey       string `json:"api_key"`
		BaseURL      string `json:"base_url"`
		SystemPrompt string `json:"system_prompt"`
		SkillsDir    string `json:"skills_dir"`
		AgentRoot    string `json:"agent_root"`
	}
	var req createBotReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.BotClient.CreateBot(ctx, client.NewCreateBotReq(req.Name, req.Type, req.Description, req.ModelName, req.APIKey, req.BaseURL, req.SystemPrompt, req.SkillsDir, req.AgentRoot, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) UpdateBot(ctx context.Context, c *app.RequestContext) {
	type updateBotReq struct {
		BotID        int64  `json:"bot_id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		ModelName    string `json:"model_name"`
		APIKey       string `json:"api_key"`
		BaseURL      string `json:"base_url"`
		SystemPrompt string `json:"system_prompt"`
		SkillsDir    string `json:"skills_dir"`
		AgentRoot    string `json:"agent_root"`
		IsActive     bool   `json:"is_active"`
	}
	var req updateBotReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.BotClient.UpdateBot(ctx, client.NewUpdateBotReq(req.BotID, id, req.Name, req.Description, req.ModelName, req.APIKey, req.BaseURL, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.IsActive))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) GetBot(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	resp, err := client.BotClient.GetBot(ctx, client.NewGetBotReq(botID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) ListBots(ctx context.Context, c *app.RequestContext) {
	userID, _ := c.Get("userID")
	id := userID.(int64)
	botType := c.DefaultQuery("type", "")
	resp, err := client.BotClient.ListBots(ctx, client.NewListBotsReq(id, botType))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) DeleteBot(ctx context.Context, c *app.RequestContext) {
	type deleteBotReq struct {
		BotID int64 `json:"bot_id"`
	}
	var req deleteBotReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.BotClient.DeleteBot(ctx, client.NewDeleteBotReq(req.BotID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) ChatWithBot(ctx context.Context, c *app.RequestContext) {
	type chatReq struct {
		BotID         int64  `json:"bot_id"`
		ConversationID int64 `json:"conversation_id"`
		Message       string `json:"message"`
	}
	var req chatReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.BotClient.ChatWithBot(ctx, client.NewChatWithBotReq(req.BotID, id, req.ConversationID, req.Message))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) CreateRoute(ctx context.Context, c *app.RequestContext) {
	type createRouteReq struct {
		BotID        int64  `json:"bot_id"`
		RoutePattern string `json:"route_pattern"`
		RouteType    string `json:"route_type"`
		Priority     int64  `json:"priority"`
	}
	var req createRouteReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	resp, err := client.BotClient.CreateRoute(ctx, client.NewCreateRouteReq(req.BotID, req.RoutePattern, req.RouteType, req.Priority))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) ListRoutes(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	resp, err := client.BotClient.ListRoutes(ctx, client.NewListRoutesReq(botID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) DeleteRoute(ctx context.Context, c *app.RequestContext) {
	type deleteRouteReq struct {
		RouteID int64 `json:"route_id"`
	}
	var req deleteRouteReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.BotClient.DeleteRoute(ctx, client.NewDeleteRouteReq(req.RouteID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) GetBilling(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	resp, err := client.BotClient.GetBilling(ctx, client.NewGetBillingReq(botID, id, limit, offset))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
