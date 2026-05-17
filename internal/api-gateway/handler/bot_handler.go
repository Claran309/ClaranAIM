// Package handler contains api-gateway HTTP handlers. BotHandler in this file
// exposes bot-manager-service through browser-facing REST endpoints and keeps
// ownership identity tied to the JWT-authenticated user.
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/pkg/response"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

// BotHandler handles bot CRUD, bot chat, bot route management and billing
// queries. It performs HTTP binding and identity extraction only; bot ownership,
// model settings and billing rules are enforced by bot-manager-service.
type BotHandler struct{}

// NewBotHandler constructs the stateless bot HTTP handler used by the router.
func NewBotHandler() *BotHandler {
	return &BotHandler{}
}

func bindBotJSONUseNumber(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

func parseBotJSONNumber(value json.Number, name string) (int64, error) {
	id, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的%s", name)
	}
	return id, nil
}

// CreateBot creates a bot owned by the current user.
//
// The gateway accepts model/provider configuration from the browser, attaches
// the JWT user ID as owner_id, and delegates validation/defaulting to
// bot-manager-service.
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
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.CreateBot(ctx, client.NewCreateBotReq(req.Name, req.Type, req.Description, req.ModelName, req.APIKey, req.BaseURL, req.SystemPrompt, req.SkillsDir, req.AgentRoot, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// UpdateBot updates bot metadata, runtime provider settings and active status.
// The operator ID is always the current JWT user so users cannot update bots on
// behalf of someone else by forging request JSON.
func (h *BotHandler) UpdateBot(ctx context.Context, c *app.RequestContext) {
	type updateBotReq struct {
		BotID        json.Number `json:"bot_id"`
		Name         string      `json:"name"`
		Description  string      `json:"description"`
		ModelName    string      `json:"model_name"`
		APIKey       string      `json:"api_key"`
		BaseURL      string      `json:"base_url"`
		SystemPrompt string      `json:"system_prompt"`
		SkillsDir    string      `json:"skills_dir"`
		AgentRoot    string      `json:"agent_root"`
		IsActive     bool        `json:"is_active"`
	}
	var req updateBotReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseBotJSONNumber(req.BotID, "Bot ID")
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.UpdateBot(ctx, client.NewUpdateBotReq(botID, id, req.Name, req.Description, req.ModelName, req.APIKey, req.BaseURL, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.IsActive))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetBot returns one bot's metadata by path ID.
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

// ListBots returns the current user's bots, optionally filtered by bot type.
func (h *BotHandler) ListBots(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	botType := c.DefaultQuery("type", "")
	resp, err := client.BotClient.ListBots(ctx, client.NewListBotsReq(id, botType))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteBot deletes one bot after bot-manager-service verifies ownership.
func (h *BotHandler) DeleteBot(ctx context.Context, c *app.RequestContext) {
	type deleteBotReq struct {
		BotID json.Number `json:"bot_id"`
	}
	var req deleteBotReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseBotJSONNumber(req.BotID, "Bot ID")
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.DeleteBot(ctx, client.NewDeleteBotReq(botID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ChatWithBot sends one user message to a bot runtime.
//
// conversation_id is passed through so bot-manager-service can scope memory by
// bot, user and conversation. A missing conversation_id means the bot service
// may use its default per-user conversation context.
func (h *BotHandler) ChatWithBot(ctx context.Context, c *app.RequestContext) {
	type chatReq struct {
		BotID          json.Number `json:"bot_id"`
		ConversationID json.Number `json:"conversation_id"`
		Message        string      `json:"message"`
	}
	var req chatReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseBotJSONNumber(req.BotID, "Bot ID")
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	var conversationID int64
	if req.ConversationID.String() != "" && req.ConversationID.String() != "0" {
		conversationID, err = strconv.ParseInt(req.ConversationID.String(), 10, 64)
		if err != nil {
			response.BadRequest(c, "无效的会话ID")
			return
		}
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.ChatWithBot(ctx, client.NewChatWithBotReq(botID, id, conversationID, req.Message))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// CreateRoute creates a routing rule for a bot. Routes are used by the future
// agent dispatch layer to map message patterns or commands to bot behavior.
func (h *BotHandler) CreateRoute(ctx context.Context, c *app.RequestContext) {
	type createRouteReq struct {
		BotID        json.Number `json:"bot_id"`
		RoutePattern string      `json:"route_pattern"`
		RouteType    string      `json:"route_type"`
		Priority     int64       `json:"priority"`
	}
	var req createRouteReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseBotJSONNumber(req.BotID, "Bot ID")
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	resp, err := client.BotClient.CreateRoute(ctx, client.NewCreateRouteReq(botID, req.RoutePattern, req.RouteType, req.Priority))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListRoutes returns all configured routing rules for one bot.
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

// DeleteRoute removes one bot routing rule after ownership checks in the bot
// service.
func (h *BotHandler) DeleteRoute(ctx context.Context, c *app.RequestContext) {
	type deleteRouteReq struct {
		RouteID json.Number `json:"route_id"`
	}
	var req deleteRouteReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	routeID, err := parseBotJSONNumber(req.RouteID, "路由ID")
	if err != nil {
		response.BadRequest(c, "无效的路由ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.DeleteRoute(ctx, client.NewDeleteRouteReq(routeID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetBilling returns paginated token/cost records for one bot and the current
// user. The service layer computes and stores the actual usage records.
func (h *BotHandler) GetBilling(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	resp, err := client.BotClient.GetBilling(ctx, client.NewGetBillingReq(botID, id, limit, offset))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
