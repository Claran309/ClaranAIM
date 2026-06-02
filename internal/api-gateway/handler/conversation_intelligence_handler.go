package handler

import (
	"ClaranAIM/pkg/conversationintelclient"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// ConversationIntelligenceHandler 暴露聊天记录智能归档接口。
// 它只做 HTTP 参数绑定和登录态裁剪；归档、消息读取、RAG 入库和候选记忆写入都由
// conversation-intelligence-service 通过内部 Kitex RPC 完成。
type ConversationIntelligenceHandler struct {
	svc conversationintelclient.Service
}

var gatewayConversationIntelligenceService conversationintelclient.Service

// InitConversationIntelligenceService 注册 conversation-intelligence-service RPC 客户端。
func InitConversationIntelligenceService(svc conversationintelclient.Service) {
	gatewayConversationIntelligenceService = svc
}

// NewConversationIntelligenceHandler 创建会话智能 HTTP handler。
func NewConversationIntelligenceHandler() *ConversationIntelligenceHandler {
	return &ConversationIntelligenceHandler{svc: gatewayConversationIntelligenceService}
}

func (h *ConversationIntelligenceHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "conversation-intelligence-service未初始化")
		return false
	}
	return true
}

// CreateDigestJob 创建一次会话归档任务。
// viewer_id 始终来自 JWT 登录态，客户端请求体里的 viewer_id 会被忽略，避免用户替别人创建可见性窗口。
func (h *ConversationIntelligenceHandler) CreateDigestJob(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	viewerID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req conversationDigestJobRequest
	if err := bindConversationIntelligenceJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	conversationID, err := parseConversationIntelligenceNumber(req.ConversationID)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "无效的会话ID")
		return
	}
	agentID, err := parseOptionalConversationIntelligenceNumber(req.AgentID)
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	startMessageID, err := parseOptionalConversationIntelligenceNumber(req.StartMessageID)
	if err != nil {
		response.BadRequest(c, "无效的起始消息ID")
		return
	}
	endMessageID, err := parseOptionalConversationIntelligenceNumber(req.EndMessageID)
	if err != nil {
		response.BadRequest(c, "无效的结束消息ID")
		return
	}
	job, err := h.svc.CreateDigestJob(ctx, conversationintelclient.CreateDigestJobInput{
		ConversationID: conversationID,
		ViewerID:       viewerID,
		AgentID:        agentID,
		StartMessageID: startMessageID,
		EndMessageID:   endMessageID,
		StartTime:      strings.TrimSpace(req.StartTime),
		EndTime:        strings.TrimSpace(req.EndTime),
		Reason:         strings.TrimSpace(req.Reason),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "job": job})
}

// ProcessDigestJob 执行归档任务，返回本次提炼出的摘要、决策、任务、主题、金句和候选记忆。
func (h *ConversationIntelligenceHandler) ProcessDigestJob(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	viewerID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	jobID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID <= 0 {
		response.BadRequest(c, "无效的归档任务ID")
		return
	}
	result, err := h.svc.ProcessDigestJob(ctx, jobID, viewerID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "job": result.Job, "artifacts": result.Artifacts})
}

// RetryDigestJob 重试当前用户拥有的失败归档任务。
func (h *ConversationIntelligenceHandler) RetryDigestJob(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	viewerID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	jobID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID <= 0 {
		response.BadRequest(c, "无效的归档任务ID")
		return
	}
	result, err := h.svc.RetryDigestJob(ctx, jobID, viewerID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "job": result.Job, "artifacts": result.Artifacts})
}

// ListDigestJobs 查询当前用户的归档任务状态，用于前端展示调度队列、失败重试和处理指标。
func (h *ConversationIntelligenceHandler) ListDigestJobs(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	viewerID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	jobs, total, err := h.svc.ListDigestJobs(ctx, conversationintelclient.ListDigestJobsInput{
		ViewerID:       viewerID,
		ConversationID: parseInt64Default(c.DefaultQuery("conversation_id", "0"), 0),
		Status:         strings.TrimSpace(c.DefaultQuery("status", "")),
		Limit:          int(parseInt64Default(c.DefaultQuery("limit", "50"), 50)),
		Offset:         int(parseInt64Default(c.DefaultQuery("offset", "0"), 0)),
	})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "jobs": jobs, "total": total})
}

// ListArtifacts 查询当前用户可见的长期会话归档产物。
func (h *ConversationIntelligenceHandler) ListArtifacts(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	viewerID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	artifacts, total, err := h.svc.ListArtifacts(ctx, conversationintelclient.ListArtifactsInput{
		ViewerID:       viewerID,
		ConversationID: parseInt64Default(c.DefaultQuery("conversation_id", "0"), 0),
		ArtifactType:   strings.TrimSpace(c.DefaultQuery("type", "")),
		Limit:          int(parseInt64Default(c.DefaultQuery("limit", "20"), 20)),
		Offset:         int(parseInt64Default(c.DefaultQuery("offset", "0"), 0)),
	})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "artifacts": artifacts, "total": total})
}

type conversationDigestJobRequest struct {
	ConversationID json.Number `json:"conversation_id"`
	ViewerID       json.Number `json:"viewer_id"`
	AgentID        json.Number `json:"agent_id"`
	StartMessageID json.Number `json:"start_message_id"`
	EndMessageID   json.Number `json:"end_message_id"`
	StartTime      string      `json:"start_time"`
	EndTime        string      `json:"end_time"`
	Reason         string      `json:"reason"`
}

func bindConversationIntelligenceJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

func parseConversationIntelligenceNumber(value json.Number) (int64, error) {
	if strings.TrimSpace(value.String()) == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(value.String(), 10, 64)
}

func parseOptionalConversationIntelligenceNumber(value json.Number) (int64, error) {
	if strings.TrimSpace(value.String()) == "" {
		return 0, nil
	}
	return strconv.ParseInt(value.String(), 10, 64)
}
