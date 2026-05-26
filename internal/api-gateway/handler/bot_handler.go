// Package handler contains api-gateway HTTP handlers. BotHandler in this file
// exposes bot-manager-service through browser-facing REST endpoints and keeps
// ownership identity tied to the JWT-authenticated user.
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/pkg/response"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

// BotHandler handles bot CRUD, bot chat, bot route management and billing
// queries. It performs HTTP binding and identity extraction only; bot ownership,
// model settings and billing rules are enforced by bot-manager-service.
type BotHandler struct{}

const agentContextHistoryLimit int64 = 80
const agentApprovalStatusPending = "pending"
const agentApprovalStatusConfirmed = "confirmed"
const agentApprovalStatusRejected = "rejected"

type agentApproval struct {
	ID             string `json:"approval_id"`
	BotID          int64  `json:"bot_id"`
	UserID         int64  `json:"user_id"`
	ConversationID int64  `json:"conversation_id"`
	Description    string `json:"description"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

var agentApprovals = struct {
	sync.Mutex
	items map[string]agentApproval
}{items: make(map[string]agentApproval)}

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

func newAgentApproval(userID, botID, conversationID int64, description string) agentApproval {
	now := time.Now()
	return agentApproval{
		ID:             fmt.Sprintf("appr_%d_%d_%d", userID, botID, now.UnixNano()),
		UserID:         userID,
		BotID:          botID,
		ConversationID: conversationID,
		Description:    strings.TrimSpace(description),
		Status:         agentApprovalStatusPending,
		CreatedAt:      now.Format(time.RFC3339),
		UpdatedAt:      now.Format(time.RFC3339),
	}
}

func storeAgentApproval(approval agentApproval) {
	agentApprovals.Lock()
	defer agentApprovals.Unlock()
	agentApprovals.items[approval.ID] = approval
}

func listAgentApprovals(userID int64) []agentApproval {
	agentApprovals.Lock()
	defer agentApprovals.Unlock()
	list := make([]agentApproval, 0)
	for _, approval := range agentApprovals.items {
		if approval.UserID == userID {
			list = append(list, approval)
		}
	}
	return list
}

func getAgentApproval(id string, userID int64) (agentApproval, bool) {
	agentApprovals.Lock()
	defer agentApprovals.Unlock()
	approval, ok := agentApprovals.items[id]
	if !ok || approval.UserID != userID {
		return agentApproval{}, false
	}
	return approval, true
}

func updateAgentApprovalStatus(id string, status string) {
	agentApprovals.Lock()
	defer agentApprovals.Unlock()
	approval, ok := agentApprovals.items[id]
	if !ok {
		return
	}
	approval.Status = status
	approval.UpdatedAt = time.Now().Format(time.RFC3339)
	agentApprovals.items[id] = approval
}

func buildApprovalConfirmationMessage(approval agentApproval, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "继续执行上一轮等待确认的操作。"
	}
	return fmt.Sprintf("用户已明确允许你继续执行上一轮等待确认的操作。\n待确认事项：%s\n用户补充说明：%s\n请在当前长会话上下文中继续执行，并在完成后给出结果、影响范围和失败风险。", approval.Description, message)
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
	resp, err := client.AgentBotClient.ChatWithBot(ctx, client.NewChatWithBotReq(botID, id, conversationID, req.Message))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if resp != nil && resp.Success && resp.Msg == "pending_user_approval" {
		approval := newAgentApproval(id, botID, conversationID, resp.Reply)
		storeAgentApproval(approval)
		response.Success(c, map[string]interface{}{
			"success":       true,
			"reply":         resp.Reply,
			"msg":           resp.Msg,
			"status":        "pending_approval",
			"approval_id":   approval.ID,
			"approval":      approval,
			"input_tokens":  resp.InputTokens,
			"output_tokens": resp.OutputTokens,
			"cost":          resp.Cost,
		})
		return
	}
	response.Success(c, resp)
}

// RunAgent is the Agent-native alias of bot chat for frontend work surfaces.
func (h *BotHandler) RunAgent(ctx context.Context, c *app.RequestContext) {
	h.ChatWithBot(ctx, c)
}

// ListAgentApprovals returns lightweight pending/handled approvals kept by this
// gateway process. It is an MVP interaction layer; durable checkpoints can
// later move this state into bot-runtime-service.
func (h *BotHandler) ListAgentApprovals(ctx context.Context, c *app.RequestContext) {
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	response.Success(c, map[string]interface{}{
		"success":   true,
		"approvals": listAgentApprovals(userID),
	})
}

// ConfirmAgentApproval continues the same Agent long session with an explicit
// user approval message, implementing the agent-ask -> user-approve -> agent-act
// flow without exposing tool internals to the browser.
func (h *BotHandler) ConfirmAgentApproval(ctx context.Context, c *app.RequestContext) {
	type confirmReq struct {
		ApprovalID string `json:"approval_id"`
		Message    string `json:"message"`
	}
	var req confirmReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	approval, ok := getAgentApproval(strings.TrimSpace(req.ApprovalID), userID)
	if !ok {
		response.NotFound(c, "审批记录不存在")
		return
	}
	if approval.Status != agentApprovalStatusPending {
		response.BadRequest(c, "审批记录已处理")
		return
	}
	updateAgentApprovalStatus(approval.ID, agentApprovalStatusConfirmed)
	resp, err := client.AgentBotClient.ChatWithBot(ctx, client.NewChatWithBotReq(approval.BotID, userID, approval.ConversationID, buildApprovalConfirmationMessage(approval, req.Message)))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// RejectAgentApproval marks one pending Agent action as rejected. The runtime is
// not invoked because rejection means the user does not want the action to run.
func (h *BotHandler) RejectAgentApproval(ctx context.Context, c *app.RequestContext) {
	type rejectReq struct {
		ApprovalID string `json:"approval_id"`
	}
	var req rejectReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	approval, ok := getAgentApproval(strings.TrimSpace(req.ApprovalID), userID)
	if !ok {
		response.NotFound(c, "审批记录不存在")
		return
	}
	updateAgentApprovalStatus(approval.ID, agentApprovalStatusRejected)
	approval.Status = agentApprovalStatusRejected
	approval.UpdatedAt = time.Now().Format(time.RFC3339)
	response.Success(c, map[string]interface{}{
		"success":  true,
		"approval": approval,
		"msg":      "已拒绝，Agent不会执行该操作",
	})
}

// AddAgentAsFriend adds the Agent's system user account to the creator's friend
// list. Bot/Agent remains a normal user identity for IM conversations.
func (h *BotHandler) AddAgentAsFriend(ctx context.Context, c *app.RequestContext) {
	type addReq struct {
		BotID   json.Number `json:"bot_id"`
		GroupID json.Number `json:"group_id"`
		Remark  string      `json:"remark"`
	}
	var req addReq
	if err := bindBotJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseBotJSONNumber(req.BotID, "Bot ID")
	if err != nil {
		response.BadRequest(c, "无效的Bot ID")
		return
	}
	groupID := int64(0)
	if strings.TrimSpace(req.GroupID.String()) != "" {
		groupID, err = strconv.ParseInt(req.GroupID.String(), 10, 64)
		if err != nil || groupID < 0 {
			response.BadRequest(c, "无效的好友分组ID")
			return
		}
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	botResp, err := client.BotClient.GetBot(ctx, client.NewGetBotReq(botID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if botResp == nil || !botResp.Success || botResp.Bot == nil {
		msg := "Agent不存在"
		if botResp != nil && botResp.Msg != "" {
			msg = botResp.Msg
		}
		response.BadRequest(c, msg)
		return
	}
	if botResp.Bot.OwnerId != userID {
		response.Forbidden(c, "只有创建者可以把该Agent加为好友")
		return
	}
	if botResp.Bot.AgentUserId <= 0 {
		response.BadRequest(c, "该Agent尚未绑定系统用户")
		return
	}
	remark := strings.TrimSpace(req.Remark)
	if remark == "" {
		remark = botResp.Bot.Name
	}
	resp, err := client.UserClient.AddFriend(ctx, client.NewAddFriendReq(userID, botResp.Bot.AgentUserId, groupID, remark))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
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
	question := strings.TrimSpace(req.Question)
	if conversationID > 0 {
		contextText, ctxErr := h.buildAgentConversationContext(ctx, conversationID, userID, agentContextHistoryLimit)
		if ctxErr != nil {
			response.Error(c, ctxErr.Error())
			return
		}
		question = mergeAgentQuestionWithContext(question, contextText)
	}
	rpcReq := client.NewAgentTaskReq(botID, userID, conversationID, question)
	var resp interface{}
	switch taskType {
	case "summary":
		resp, err = client.AgentBotClient.SummarizeConversation(ctx, rpcReq)
	case "ask":
		resp, err = client.AgentBotClient.AskConversation(ctx, rpcReq)
	case "insights":
		resp, err = client.AgentBotClient.ExtractInsights(ctx, rpcReq)
	case "reply_candidates":
		resp, err = client.AgentBotClient.GenerateReplyCandidates(ctx, rpcReq)
	}
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *BotHandler) buildAgentConversationContext(ctx context.Context, conversationID, userID, limit int64) (string, error) {
	historyResp, err := client.MessageClient.GetHistory(ctx, client.NewGetHistoryReq(conversationID, userID, limit, 0))
	if err != nil {
		return "", err
	}
	if historyResp == nil || !historyResp.Success {
		msg := "读取会话历史失败"
		if historyResp != nil && historyResp.Msg != "" {
			msg = historyResp.Msg
		}
		return "", fmt.Errorf("%s", msg)
	}
	return formatMessagesForAgentContext(historyResp.Messages), nil
}

func mergeAgentQuestionWithContext(question, contextText string) string {
	if strings.TrimSpace(contextText) == "" {
		contextText = "（未读取到当前用户可见的历史消息。请明确告诉用户：当前没有可总结的会话内容。）"
	}
	if strings.TrimSpace(question) == "" {
		question = "请基于上面的会话材料完成任务。"
	}
	return fmt.Sprintf("用户补充要求：\n%s\n\n会话材料说明：下面是系统从当前会话读取到的历史消息，已按时间从旧到新排列，且都属于当前用户可见范围。请把这些消息当作唯一事实来源。\n\n会话材料：\n%s", question, contextText)
}

func formatMessagesForAgentContext(messages []*message.Message) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			content = fmt.Sprintf("[%s消息]", msg.MsgType)
		}
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.ReplaceAll(content, "\n", " ")
		if len([]rune(content)) > 600 {
			content = string([]rune(content)[:600]) + "..."
		}
		fmt.Fprintf(&b, "- [%s] 用户%d: %s\n", msg.CreatedAt, msg.SenderId, content)
	}
	return strings.TrimSpace(b.String())
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
