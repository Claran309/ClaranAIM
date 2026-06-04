// Package handler 包含 api-gateway 的 HTTP 处理器。
// 本文件把 agent-manager-service 暴露为浏览器可用的 REST 接口，并始终使用 JWT 中的用户身份作为操作者，
// 避免前端伪造 owner_id、operator_id 等敏感字段。
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/pkg/response"
	"ClaranAIM/pkg/settingsclient"
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

// AgentHandler 处理 Agent 创建、编辑、对话、路由、权限和计费查询。
// 这里仅做 HTTP 参数绑定、登录态提取和响应适配；Agent 归属、模型配置、计费和权限规则由 agent-manager-service 执行。
type AgentHandler struct{}

// defaultAgentContextHistoryLimit 是读取会话上下文的兜底条数。
// 正常情况下该值来自 Agent 配置；只有读取配置失败或旧数据为空时才使用这里的默认值。
const defaultAgentContextHistoryLimit int64 = 80

// agentApprovalStatusPending 表示 Agent 工具动作正在等待用户确认。
const agentApprovalStatusPending = "pending"

// agentApprovalStatusConfirmed 表示用户已经允许 Agent 继续执行等待中的动作。
const agentApprovalStatusConfirmed = "confirmed"

// agentApprovalStatusRejected 表示用户拒绝了 Agent 的待执行动作。
const agentApprovalStatusRejected = "rejected"

// agentApproval 是网关内存中的轻量审批记录，用于 MVP 版本的 agent-ask -> user-approve -> agent-act 交互。
// 后续如果要支持多实例网关或跨重启恢复，应迁移到 agent-runtime-service 的持久化 checkpoint。
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

// agentApprovals 是网关进程内的轻量审批存储。
// 这是单实例 MVP，跨实例或跨重启可靠性应迁移到 runtime checkpoint 表。
var agentApprovals = struct {
	sync.Mutex
	items map[string]agentApproval
}{items: make(map[string]agentApproval)}

// gatewayAgentSettingsService 用于创建 Agent 时解析用户保存的 LLM 预设。
// 网关只拿服务间密钥结果，不把 API Key 明文返回给浏览器。
var gatewayAgentSettingsService settingsclient.Service

// InitAgentSettingsService 注入 settings-service 客户端。
// 创建 Agent 时可以引用用户保存的 LLM profile，但浏览器永远拿不到 profile 中的 API Key 明文。
func InitAgentSettingsService(svc settingsclient.Service) {
	gatewayAgentSettingsService = svc
}

// NewAgentHandler 创建无状态 Agent HTTP handler，供 router 注册路由。
func NewAgentHandler() *AgentHandler {
	return &AgentHandler{}
}

// bindAgentJSONUseNumber 使用 json.Number 解码请求体，避免大整数 ID 被 float64 精度截断。
func bindAgentJSONUseNumber(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

// parseAgentJSONNumber 将 JSON 数字字段解析为正 int64，并生成面向业务的错误文案。
func parseAgentJSONNumber(value json.Number, name string) (int64, error) {
	id, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的%s", name)
	}
	return id, nil
}

// newAgentApproval 创建一条待用户确认的 Agent 操作记录。
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

// storeAgentApproval 将审批记录写入当前网关进程内存。
func storeAgentApproval(approval agentApproval) {
	agentApprovals.Lock()
	defer agentApprovals.Unlock()
	agentApprovals.items[approval.ID] = approval
}

// listAgentApprovals 列出当前用户相关的审批记录。
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

// getAgentApproval 按审批 ID 和当前用户读取记录，防止用户查看或确认别人的审批。
func getAgentApproval(id string, userID int64) (agentApproval, bool) {
	agentApprovals.Lock()
	defer agentApprovals.Unlock()
	approval, ok := agentApprovals.items[id]
	if !ok || approval.UserID != userID {
		return agentApproval{}, false
	}
	return approval, true
}

// updateAgentApprovalStatus 更新审批状态和更新时间。
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

// buildApprovalConfirmationMessage 将用户确认动作转成 Agent 可继续执行的自然语言指令。
func buildApprovalConfirmationMessage(approval agentApproval, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "继续执行上一轮等待确认的操作。"
	}
	return fmt.Sprintf("用户已明确允许你继续执行上一轮等待确认的操作。\n待确认事项：%s\n用户补充说明：%s\n请在当前长会话上下文中继续执行，并在完成后给出结果、影响范围和失败风险。", approval.Description, message)
}

// CreateAgent 创建属于当前用户的 Agent。
// 网关接收浏览器传来的模型/供应商配置，并把 JWT 用户 ID 注入为 owner_id；
// 具体校验、默认值和系统用户创建由 agent-manager-service 负责。
// 生成代码里仍有历史 bot 命名，是因为 Kitex IDL 尚未安全重生成到 agent 包名。
func (h *AgentHandler) CreateAgent(ctx context.Context, c *app.RequestContext) {
	type createBotReq struct {
		Name                string  `json:"name"`
		Type                string  `json:"type"`
		Description         string  `json:"description"`
		ModelName           string  `json:"model_name"`
		APIKey              string  `json:"api_key"`
		BaseURL             string  `json:"base_url"`
		SystemPrompt        string  `json:"system_prompt"`
		SkillsDir           string  `json:"skills_dir"`
		AgentRoot           string  `json:"agent_root"`
		Avatar              string  `json:"avatar"`
		Signature           string  `json:"signature"`
		WorkspaceRoot       string  `json:"workspace_root"`
		ToolPolicy          string  `json:"tool_policy"`
		LLMProfileID        int64   `json:"llm_profile_id"`
		ContextMessageLimit int64   `json:"context_message_limit"`
		MemoryRecallLimit   int64   `json:"memory_recall_limit"`
		MaxOutputTokens     int64   `json:"max_output_tokens"`
		Temperature         float64 `json:"temperature"`
		GroupTriggerMode    string  `json:"group_trigger_mode"`
		AutoReplyEnabled    bool    `json:"auto_reply_enabled"`
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
	if req.LLMProfileID > 0 {
		if gatewayAgentSettingsService == nil {
			response.Error(c, "settings-service未初始化")
			return
		}
		profile, err := gatewayAgentSettingsService.ResolveLLMProfile(ctx, id, req.LLMProfileID)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		req.Type = "custom"
		req.APIKey = profile.APIKey
		req.BaseURL = profile.BaseURL
		req.ModelName = profile.ModelName
	}
	resp, err := client.AgentClient.CreateBot(ctx, client.NewCreateAgentReq(req.Name, req.Type, req.Description, req.ModelName, req.APIKey, req.BaseURL, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.Avatar, req.Signature, req.WorkspaceRoot, req.ToolPolicy, id, req.ContextMessageLimit, req.MemoryRecallLimit, req.MaxOutputTokens, req.Temperature, req.GroupTriggerMode, req.AutoReplyEnabled))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// UpdateAgent 更新 Agent 元数据、运行时供应商配置和启停状态。
// operatorID 始终来自 JWT 当前用户，防止用户通过伪造 JSON 替别人修改 Agent。
func (h *AgentHandler) UpdateAgent(ctx context.Context, c *app.RequestContext) {
	type updateBotReq struct {
		BotID               json.Number `json:"bot_id"`
		Name                string      `json:"name"`
		Description         string      `json:"description"`
		ModelName           string      `json:"model_name"`
		APIKey              string      `json:"api_key"`
		BaseURL             string      `json:"base_url"`
		SystemPrompt        string      `json:"system_prompt"`
		SkillsDir           string      `json:"skills_dir"`
		AgentRoot           string      `json:"agent_root"`
		Avatar              string      `json:"avatar"`
		Signature           string      `json:"signature"`
		WorkspaceRoot       string      `json:"workspace_root"`
		ToolPolicy          string      `json:"tool_policy"`
		IsActive            bool        `json:"is_active"`
		ContextMessageLimit int64       `json:"context_message_limit"`
		MemoryRecallLimit   int64       `json:"memory_recall_limit"`
		MaxOutputTokens     int64       `json:"max_output_tokens"`
		Temperature         float64     `json:"temperature"`
		GroupTriggerMode    string      `json:"group_trigger_mode"`
		AutoReplyEnabled    bool        `json:"auto_reply_enabled"`
		LLMProfileID        int64       `json:"llm_profile_id"`
	}
	var raw map[string]json.RawMessage
	if err := bindAgentJSONUseNumber(c, &raw); err != nil {
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
	_, autoReplySet := raw["auto_reply_enabled"]
	if !autoReplySet {
		req.AutoReplyEnabled = true
	}
	botID, err := parseAgentJSONNumber(req.BotID, "Agent ID")
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	if req.LLMProfileID > 0 {
		if gatewayAgentSettingsService == nil {
			response.Error(c, "settings-service未初始化")
			return
		}
		profile, err := gatewayAgentSettingsService.ResolveLLMProfile(ctx, id, req.LLMProfileID)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		req.APIKey = profile.APIKey
		req.BaseURL = profile.BaseURL
		req.ModelName = profile.ModelName
	}
	resp, err := client.AgentClient.UpdateBot(ctx, client.NewUpdateAgentReq(botID, id, req.Name, req.Description, req.ModelName, req.APIKey, req.BaseURL, req.SystemPrompt, req.SkillsDir, req.AgentRoot, req.Avatar, req.Signature, req.WorkspaceRoot, req.ToolPolicy, req.IsActive, isActiveSet, req.ContextMessageLimit, req.MemoryRecallLimit, req.MaxOutputTokens, req.Temperature, req.GroupTriggerMode, req.AutoReplyEnabled))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetAgent 根据路径 ID 查询单个 Agent 元数据。
func (h *AgentHandler) GetAgent(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	resp, err := client.AgentClient.GetBot(ctx, client.NewGetAgentReq(botID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListAgents 查询当前用户拥有或可见的 Agent，可按类型过滤。
func (h *AgentHandler) ListAgents(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	botType := c.DefaultQuery("type", "")
	resp, err := client.AgentClient.ListBots(ctx, client.NewListAgentsReq(id, botType))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteAgent 删除 Agent；是否有权删除由 agent-manager-service 复核。
func (h *AgentHandler) DeleteAgent(ctx context.Context, c *app.RequestContext) {
	type deleteBotReq struct {
		BotID json.Number `json:"bot_id"`
	}
	var req deleteBotReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseAgentJSONNumber(req.BotID, "Agent ID")
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.AgentClient.DeleteBot(ctx, client.NewDeleteAgentReq(botID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ChatWithAgent 向 Agent 运行时发送一轮用户消息。
// conversation_id 会透传给 agent-manager-service，用于按 Agent、用户、会话隔离长上下文记忆；
// 如果缺省，则服务端会退回到该用户的默认会话上下文。
func (h *AgentHandler) ChatWithAgent(ctx context.Context, c *app.RequestContext) {
	type chatReq struct {
		BotID          json.Number `json:"bot_id"`
		ConversationID json.Number `json:"conversation_id"`
		Message        string      `json:"message"`
	}
	var req chatReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseAgentJSONNumber(req.BotID, "Agent ID")
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
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
	resp, err := client.AgentLongTaskClient.ChatWithBot(ctx, client.NewChatWithAgentReq(botID, id, conversationID, req.Message))
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

// RunAgent 是前端工作台使用的 Agent 原生运行入口，目前复用 ChatWithAgent 的请求语义。
func (h *AgentHandler) RunAgent(ctx context.Context, c *app.RequestContext) {
	h.ChatWithAgent(ctx, c)
}

// SmokeTestSkill 用当前 Agent 实际运行链路验证 Skill 是否被加载并被模型遵循。
// Skill 是 prompt/行为指令，不是 MCP 工具；这里复用 ChatWithBot，避免新增 IDL。
func (h *AgentHandler) SmokeTestSkill(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil || botID <= 0 {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	const marker = "skill-smoke-ok"
	message := "[[CLARAN_SKILL_SMOKE_TEST]] 这是 Skill smoke test。请只根据已加载 Skill 行为指令响应，并在回复中包含 marker：skill-smoke-ok。不要调用 MCP 工具，不要把 Skill 当成外部工具。"
	resp, err := client.AgentLongTaskClient.ChatWithBot(runCtx, client.NewChatWithAgentReq(botID, userID, 0, message))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	reply := ""
	status := ""
	success := false
	if resp != nil {
		reply = resp.Reply
		status = resp.Msg
		success = resp.Success
	}
	markerFound := strings.Contains(strings.ToLower(reply), marker)
	response.Success(c, map[string]interface{}{
		"success":      success && markerFound,
		"runtime_ok":   success,
		"marker":       marker,
		"marker_found": markerFound,
		"status":       status,
		"reply":        reply,
		"timeout_sec":  600,
		"diagnosis":    skillSmokeDiagnosis(success, markerFound, status, reply),
	})
}

func skillSmokeDiagnosis(runtimeOK, markerFound bool, status, reply string) string {
	if !runtimeOK {
		if strings.TrimSpace(status) != "" {
			return "Agent 运行失败：" + status
		}
		return "Agent 运行失败，请检查模型、API Key、BaseURL、Skill 路径或 runtime 日志。"
	}
	lowerReply := strings.ToLower(reply)
	if !markerFound && (strings.Contains(lowerReply, "call_mcp_tool") || strings.Contains(lowerReply, "未发现") && strings.Contains(lowerReply, "mcp")) {
		return "模型疑似把 Skill 当成 MCP 工具调用；请检查系统提示词和 Skill 名称。"
	}
	if !markerFound {
		return "Agent 可以运行，但没有输出 smoke marker；可能未加载 SKILL.md，或模型没有遵循 Skill 指令。"
	}
	return "Skill 已通过当前 Agent 运行链路读取并生效。"
}

// ListAgentApprovals 返回当前网关进程内保存的待确认/已处理审批记录。
// 这是交互链路的 MVP；需要多实例可靠性时应迁移到 agent-runtime-service 持久化。
func (h *AgentHandler) ListAgentApprovals(ctx context.Context, c *app.RequestContext) {
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	response.Success(c, map[string]interface{}{
		"success":   true,
		"approvals": listAgentApprovals(userID),
	})
}

// ConfirmAgentApproval 用用户的明确确认继续同一个 Agent 长会话。
// 这样前端可以实现 agent-ask -> user-approve -> agent-act，而不需要暴露工具内部参数。
func (h *AgentHandler) ConfirmAgentApproval(ctx context.Context, c *app.RequestContext) {
	type confirmReq struct {
		ApprovalID string `json:"approval_id"`
		Message    string `json:"message"`
	}
	var req confirmReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
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
	resp, err := client.AgentLongTaskClient.ChatWithBot(ctx, client.NewChatWithAgentReq(approval.BotID, userID, approval.ConversationID, buildApprovalConfirmationMessage(approval, req.Message)))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// RejectAgentApproval 将待执行动作标记为拒绝。
// 拒绝意味着用户不希望执行该工具动作，因此不会再调用 runtime。
func (h *AgentHandler) RejectAgentApproval(ctx context.Context, c *app.RequestContext) {
	type rejectReq struct {
		ApprovalID string `json:"approval_id"`
	}
	var req rejectReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
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

// AddAgentAsFriend 将 Agent 绑定的系统用户加入创建者好友列表。
// Agent 在 IM 中仍是普通用户身份，可被私聊、入群和 @。
func (h *AgentHandler) AddAgentAsFriend(ctx context.Context, c *app.RequestContext) {
	type addReq struct {
		BotID   json.Number `json:"bot_id"`
		GroupID json.Number `json:"group_id"`
		Remark  string      `json:"remark"`
	}
	var req addReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseAgentJSONNumber(req.BotID, "Agent ID")
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
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
	botResp, err := client.AgentClient.GetBot(ctx, client.NewGetAgentReq(botID))
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

// SummarizeConversation 触发“总结当前会话”任务，实际执行由 Agent 运行时完成。
func (h *AgentHandler) SummarizeConversation(ctx context.Context, c *app.RequestContext) {
	h.agentTask(ctx, c, "summary")
}

// AskConversation 触发基于当前会话上下文的问答任务。
func (h *AgentHandler) AskConversation(ctx context.Context, c *app.RequestContext) {
	h.agentTask(ctx, c, "ask")
}

// ExtractInsights 触发会话洞察提取任务，例如结论、风险、待办和负责人。
func (h *AgentHandler) ExtractInsights(ctx context.Context, c *app.RequestContext) {
	h.agentTask(ctx, c, "insights")
}

// GenerateReplyCandidates 触发候选回复生成任务，供用户在聊天框中选择或编辑。
func (h *AgentHandler) GenerateReplyCandidates(ctx context.Context, c *app.RequestContext) {
	h.agentTask(ctx, c, "reply_candidates")
}

// agentTask 统一处理总结、问答、洞察、候选回复这类“带会话上下文”的 Agent 任务。
func (h *AgentHandler) agentTask(ctx context.Context, c *app.RequestContext, taskType string) {
	type taskReq struct {
		BotID          json.Number `json:"bot_id"`
		ConversationID json.Number `json:"conversation_id"`
		Question       string      `json:"question"`
	}
	var req taskReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseAgentJSONNumber(req.BotID, "Agent ID")
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
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
		contextLimit := h.resolveAgentContextLimit(ctx, botID)
		contextText, ctxErr := h.buildAgentConversationContext(ctx, conversationID, userID, contextLimit)
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
		resp, err = client.AgentLongTaskClient.SummarizeConversation(ctx, rpcReq)
	case "ask":
		resp, err = client.AgentLongTaskClient.AskConversation(ctx, rpcReq)
	case "insights":
		resp, err = client.AgentLongTaskClient.ExtractInsights(ctx, rpcReq)
	case "reply_candidates":
		resp, err = client.AgentLongTaskClient.GenerateReplyCandidates(ctx, rpcReq)
	}
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// resolveAgentContextLimit 从 Agent 配置中读取上下文消息数量。
// 读取失败时退回默认值，避免“总结会话”因为配置接口临时异常完全不可用。
func (h *AgentHandler) resolveAgentContextLimit(ctx context.Context, botID int64) int64 {
	resp, err := client.AgentClient.GetBot(ctx, client.NewGetAgentReq(botID))
	if err != nil || resp == nil || !resp.Success || resp.Bot == nil || resp.Bot.ContextMessageLimit <= 0 {
		return defaultAgentContextHistoryLimit
	}
	return resp.Bot.ContextMessageLimit
}

// buildAgentConversationContext 从 msg-core-service 读取当前用户可见的会话历史，并格式化为 Agent 上下文。
func (h *AgentHandler) buildAgentConversationContext(ctx context.Context, conversationID, userID, limit int64) (string, error) {
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

// mergeAgentQuestionWithContext 将用户问题与服务端读取到的会话材料合并，明确要求 Agent 只基于可见历史回答。
func mergeAgentQuestionWithContext(question, contextText string) string {
	if strings.TrimSpace(contextText) == "" {
		contextText = "（未读取到当前用户可见的历史消息。请明确告诉用户：当前没有可总结的会话内容。）"
	}
	if strings.TrimSpace(question) == "" {
		question = "请基于上面的会话材料完成任务。"
	}
	return fmt.Sprintf("用户补充要求：\n%s\n\n会话材料说明：下面是系统从当前会话读取到的历史消息，已按时间从旧到新排列，且都属于当前用户可见范围。请把这些消息当作唯一事实来源。\n\n会话材料：\n%s", question, contextText)
}

// formatMessagesForAgentContext 将消息列表压缩成按时间排列的纯文本上下文，避免把过长消息直接塞爆模型输入。
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

// GrantPermission 给其他用户授予 Agent 协作角色。
func (h *AgentHandler) GrantPermission(ctx context.Context, c *app.RequestContext) {
	type grantReq struct {
		BotID  json.Number `json:"bot_id"`
		UserID json.Number `json:"user_id"`
		Role   string      `json:"role"`
	}
	var req grantReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseAgentJSONNumber(req.BotID, "Agent ID")
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	userID, err := parseAgentJSONNumber(req.UserID, "用户ID")
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	operatorID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.AgentClient.GrantPermission(ctx, client.NewGrantPermissionReq(botID, operatorID, userID, req.Role))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// RevokePermission 撤销其他用户的 Agent 协作角色。
func (h *AgentHandler) RevokePermission(ctx context.Context, c *app.RequestContext) {
	type revokeReq struct {
		BotID  json.Number `json:"bot_id"`
		UserID json.Number `json:"user_id"`
	}
	var req revokeReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseAgentJSONNumber(req.BotID, "Agent ID")
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	userID, err := parseAgentJSONNumber(req.UserID, "用户ID")
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	operatorID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.AgentClient.RevokePermission(ctx, client.NewRevokePermissionReq(botID, operatorID, userID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListPermissions 查询某个 Agent 的协作者角色列表。
func (h *AgentHandler) ListPermissions(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	operatorID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.AgentClient.ListPermissions(ctx, client.NewListPermissionsReq(botID, operatorID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListAgentSessions 暴露 agent-runtime-service 持久化的长会话元数据。
// 网关会先通过 agent-manager-service 做权限检查，避免把 session ID 泄露给无查看权限的用户。
func (h *AgentHandler) ListAgentSessions(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	permResp, err := client.AgentClient.ListPermissions(ctx, client.NewListPermissionsReq(botID, userID))
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
	conversationID, ok := parseNonNegativeQueryInt64(c, "conversation_id", 0)
	if !ok {
		return
	}
	resp, err := client.AgentRuntimeClient.GetAgentSessions(ctx, &bot_runtime.GetAgentSessionReq{
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

// CreateRoute 创建 Agent 路由规则。
// 路由规则供 Agent Dispatcher 把消息模式、命令或静默记录策略映射到具体行为。
func (h *AgentHandler) CreateRoute(ctx context.Context, c *app.RequestContext) {
	type createRouteReq struct {
		BotID        json.Number `json:"bot_id"`
		RoutePattern string      `json:"route_pattern"`
		RouteType    string      `json:"route_type"`
		Priority     int64       `json:"priority"`
	}
	var req createRouteReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseAgentJSONNumber(req.BotID, "Agent ID")
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	resp, err := client.AgentClient.CreateRoute(ctx, client.NewCreateRouteReq(botID, req.RoutePattern, req.RouteType, req.Priority))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListRoutes 查询某个 Agent 的全部路由规则。
func (h *AgentHandler) ListRoutes(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	resp, err := client.AgentClient.ListRoutes(ctx, client.NewListRoutesReq(botID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteRoute 删除一条 Agent 路由规则，权限由下游服务复核。
func (h *AgentHandler) DeleteRoute(ctx context.Context, c *app.RequestContext) {
	type deleteRouteReq struct {
		RouteID json.Number `json:"route_id"`
	}
	var req deleteRouteReq
	if err := bindAgentJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	routeID, err := parseAgentJSONNumber(req.RouteID, "路由ID")
	if err != nil {
		response.BadRequest(c, "无效的路由ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.AgentClient.DeleteRoute(ctx, client.NewDeleteRouteReq(routeID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetBilling 查询某个 Agent 对当前用户产生的 token 和费用记录。
// 实际 token 用量和费用由服务层根据模型返回 usage 记录。
func (h *AgentHandler) GetBilling(ctx context.Context, c *app.RequestContext) {
	botIDStr := c.Param("id")
	botID, err := strconv.ParseInt(botIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	limit, ok := parsePositiveLimit(c, "limit", 20, 100)
	if !ok {
		return
	}
	offset, ok := parseNonNegativeQueryInt64(c, "offset", 0)
	if !ok {
		return
	}
	resp, err := client.AgentClient.GetBilling(ctx, client.NewGetBillingReq(botID, id, limit, offset))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
