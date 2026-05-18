// Package handler contains api-gateway HTTP handlers. BotHandler in this file
// exposes bot-manager-service through browser-facing REST endpoints and keeps
// ownership identity tied to the JWT-authenticated user.
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/bot_runtime"
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
		Name          string `json:"name"`
		Type          string `json:"type"`
		Description   string `json:"description"`
		ModelName     string `json:"model_name"`
		APIKey        string `json:"api_key"`
		BaseURL       string `json:"base_url"`
		SystemPrompt  string `json:"system_prompt"`
		SkillsDir     string `json:"skills_dir"`
		AgentRoot     string `json:"agent_root"`
		Avatar        string `json:"avatar"`
		Signature     string `json:"signature"`
		WorkspaceRoot string `json:"workspace_root"`
		ToolPolicy    string `json:"tool_policy"`
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
	resp, err := client.BotClient.CreateBot(ctx, client.NewCreateBotReq(req.Name, req.Type, req.Description, req.ModelName, req.APIKey, req.BaseURL, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.Avatar, req.Signature, req.WorkspaceRoot, req.ToolPolicy, id))
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
		BotID         json.Number `json:"bot_id"`
		Name          string      `json:"name"`
		Description   string      `json:"description"`
		ModelName     string      `json:"model_name"`
		APIKey        string      `json:"api_key"`
		BaseURL       string      `json:"base_url"`
		SystemPrompt  string      `json:"system_prompt"`
		SkillsDir     string      `json:"skills_dir"`
		AgentRoot     string      `json:"agent_root"`
		Avatar        string      `json:"avatar"`
		Signature     string      `json:"signature"`
		WorkspaceRoot string      `json:"workspace_root"`
		ToolPolicy    string      `json:"tool_policy"`
		IsActive      bool        `json:"is_active"`
	}
	var raw map[string]json.RawMessage
	if err := bindBotJSONUseNumber(c, &raw); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	var req updateBotReq
	bodyCopy, _ := json.Marshal(raw)
	decoder := json.NewDecoder(bytes.NewReader(bodyCopy))
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	_, isActiveSet := raw["is_active"]
	botID, err := parseBotJSONNumber(req.BotID, "Bot ID")
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.UpdateBot(ctx, client.NewUpdateBotReq(botID, id, req.Name, req.Description, req.ModelName, req.APIKey, req.BaseURL, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.Avatar, req.Signature, req.WorkspaceRoot, req.ToolPolicy, req.IsActive, isActiveSet))
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

// RunAgent is the Agent-native alias of bot chat for frontend work surfaces.
func (h *BotHandler) RunAgent(ctx context.Context, c *app.RequestContext) {
	h.ChatWithBot(ctx, c)
}

func (h *BotHandler) SummarizeConversation(ctx context.Context, c *app.RequestContext) {
	h.agentTask(ctx, c, "summary")
}

func (h *BotHandler) AskConversation(ctx context.Context, c *app.RequestContext) {
	h.agentTask(ctx, c, "ask")
}

func (h *BotHandler) ExtractInsights(ctx context.Context, c *app.RequestContext) {
	h.agentTask(ctx, c, "insights")
}

func (h *BotHandler) GenerateReplyCandidates(ctx context.Context, c *app.RequestContext) {
	h.agentTask(ctx, c, "reply_candidates")
}

func (h *BotHandler) agentTask(ctx context.Context, c *app.RequestContext, taskType string) {
	type taskReq struct {
		BotID          json.Number `json:"bot_id"`
		ConversationID json.Number `json:"conversation_id"`
		Question       string      `json:"question"`
	}
	var req taskReq
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
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	rpcReq := client.NewAgentTaskReq(botID, userID, conversationID, req.Question)
	var resp interface{}
	switch taskType {
	case "summary":
		resp, err = client.BotClient.SummarizeConversation(ctx, rpcReq)
	case "ask":
		resp, err = client.BotClient.AskConversation(ctx, rpcReq)
	case "insights":
		resp, err = client.BotClient.ExtractInsights(ctx, rpcReq)
	case "reply_candidates":
		resp, err = client.BotClient.GenerateReplyCandidates(ctx, rpcReq)
	}
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GrantPermission grants another user a role on an Agent.
func (h *BotHandler) GrantPermission(ctx context.Context, c *app.RequestContext) {
	type grantReq struct {
		BotID  json.Number `json:"bot_id"`
		UserID json.Number `json:"user_id"`
		Role   string      `json:"role"`
	}
	var req grantReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseBotJSONNumber(req.BotID, "Bot ID")
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	userID, err := parseBotJSONNumber(req.UserID, "用户ID")
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	operatorID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.GrantPermission(ctx, client.NewGrantPermissionReq(botID, operatorID, userID, req.Role))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// RevokePermission revokes another user's Agent role.
func (h *BotHandler) RevokePermission(ctx context.Context, c *app.RequestContext) {
	type revokeReq struct {
		BotID  json.Number `json:"bot_id"`
		UserID json.Number `json:"user_id"`
	}
	var req revokeReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseBotJSONNumber(req.BotID, "Bot ID")
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	userID, err := parseBotJSONNumber(req.UserID, "用户ID")
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	operatorID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.RevokePermission(ctx, client.NewRevokePermissionReq(botID, operatorID, userID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListPermissions returns all roles for one Agent.
func (h *BotHandler) ListPermissions(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	operatorID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.BotClient.ListPermissions(ctx, client.NewListPermissionsReq(botID, operatorID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListAgentSessions exposes bot-runtime-service persisted long-session metadata.
// The gateway first asks bot-manager-service for permissions so session IDs are
// not leaked to users who cannot at least view the Agent.
func (h *BotHandler) ListAgentSessions(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	permResp, err := client.BotClient.ListPermissions(ctx, client.NewListPermissionsReq(botID, userID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if permResp == nil || !permResp.Success {
		msg := "无权查看智能助手会话"
		if permResp != nil && permResp.Msg != "" {
			msg = permResp.Msg
		}
		response.Forbidden(c, msg)
		return
	}
	conversationID, _ := strconv.ParseInt(c.DefaultQuery("conversation_id", "0"), 10, 64)
	resp, err := client.BotRuntimeClient.GetAgentSessions(ctx, &bot_runtime.GetAgentSessionReq{
		BotId:          botID,
		UserId:         userID,
		ConversationId: conversationID,
	})
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
