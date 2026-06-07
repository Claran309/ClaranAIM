package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/conversation_intelligence"
	"ClaranAIM/kitex_gen/conversation_intelligence/conversationintelligenceservice"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// ConversationIntelligenceHandler 暴露聊天记录智能归档接口。
// 它只做 HTTP 参数绑定和登录态裁剪；归档、消息读取、RAG 入库和候选记忆写入都由
// conversation-intelligence-service 通过内部 Kitex RPC 完成。
type ConversationIntelligenceHandler struct {
	svc conversationintelligenceservice.Client
}

var gatewayConversationIntelligenceService conversationintelligenceservice.Client

// InitConversationIntelligenceService 注册 conversation-intelligence-service RPC 客户端。
func InitConversationIntelligenceService(svc conversationintelligenceservice.Client) {
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
	resp, err := h.svc.CreateDigestJob(ctx, &conversation_intelligence.CreateDigestJobReq{
		ConversationId: conversationID,
		ViewerId:       viewerID,
		AgentId:        agentID,
		StartMessageId: startMessageID,
		EndMessageId:   endMessageID,
		StartTime:      strings.TrimSpace(req.StartTime),
		EndTime:        strings.TrimSpace(req.EndTime),
		Reason:         strings.TrimSpace(req.Reason),
	})
	if err != nil || !resp.GetSuccess() {
		if err == nil {
			err = conversationIntelStatusError(resp.GetSuccess(), resp.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "job": resp.GetJob()})
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
	result, err := h.svc.ProcessDigestJob(ctx, &conversation_intelligence.ProcessDigestJobReq{JobId: jobID, ViewerId: viewerID})
	if err != nil || !result.GetSuccess() {
		if err == nil {
			err = conversationIntelStatusError(result.GetSuccess(), result.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "job": result.GetJob(), "artifacts": result.GetArtifacts()})
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
	result, err := h.svc.RetryDigestJob(ctx, &conversation_intelligence.RetryDigestJobReq{JobId: jobID, ViewerId: viewerID})
	if err != nil || !result.GetSuccess() {
		if err == nil {
			err = conversationIntelStatusError(result.GetSuccess(), result.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "job": result.GetJob(), "artifacts": result.GetArtifacts()})
}

// MissedSummary 只摘要当前登录用户在指定会话里尚未读过的消息范围。
// 它不会推进已读游标；用户确认看完后仍需显式调用 MarkConversationRead。
func (h *ConversationIntelligenceHandler) MissedSummary(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	viewerID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req conversationMissedSummaryRequest
	if err := bindConversationIntelligenceJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	conversationID, err := parseConversationIntelligenceNumber(req.ConversationID)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "无效的会话ID")
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 120
	}
	if limit > 300 {
		limit = 300
	}
	history, err := client.MessageClient.GetHistory(ctx, client.NewGetHistoryReq(conversationID, viewerID, int64(limit), 0))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if history == nil || !history.GetSuccess() {
		msg := ""
		if history != nil {
			msg = history.GetMsg()
		}
		response.BadRequest(c, errorMessage(nil, msg))
		return
	}
	unread := unreadMessagesFromHistory(history.GetMessages())
	if len(unread) == 0 {
		response.Success(c, map[string]interface{}{
			"success":         true,
			"empty":           true,
			"conversation_id": conversationID,
			"message_count":   0,
			"msg":             "当前没有未读消息",
			"artifacts":       []*conversation_intelligence.ConversationArtifact{},
		})
		return
	}
	sort.Slice(unread, func(i, j int) bool { return unread[i].GetId() < unread[j].GetId() })
	startID := unread[0].GetId()
	endID := unread[len(unread)-1].GetId()
	jobResp, err := h.svc.CreateDigestJob(ctx, &conversation_intelligence.CreateDigestJobReq{
		ConversationId: conversationID,
		ViewerId:       viewerID,
		StartMessageId: startID,
		EndMessageId:   endID,
		Reason:         "missed_unread_summary",
	})
	if err != nil || !jobResp.GetSuccess() {
		if err == nil {
			err = conversationIntelStatusError(jobResp.GetSuccess(), jobResp.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.svc.ProcessDigestJob(ctx, &conversation_intelligence.ProcessDigestJobReq{JobId: jobResp.GetJob().GetId(), ViewerId: viewerID})
	if err != nil || !result.GetSuccess() {
		if err == nil {
			err = conversationIntelStatusError(result.GetSuccess(), result.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{
		"success":           true,
		"empty":             false,
		"conversation_id":   conversationID,
		"start_message_id":  startID,
		"end_message_id":    endID,
		"message_count":     len(unread),
		"job":               result.GetJob(),
		"artifacts":         result.GetArtifacts(),
		"mark_read_message": endID,
	})
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
	resp, err := h.svc.ListDigestJobs(ctx, &conversation_intelligence.ListDigestJobsReq{
		ViewerId:       viewerID,
		ConversationId: parseInt64Default(c.DefaultQuery("conversation_id", "0"), 0),
		Status:         strings.TrimSpace(c.DefaultQuery("status", "")),
		Limit:          parseInt64Default(c.DefaultQuery("limit", "50"), 50),
		Offset:         parseInt64Default(c.DefaultQuery("offset", "0"), 0),
	})
	if err != nil || !resp.GetSuccess() {
		if err == nil {
			err = conversationIntelStatusError(resp.GetSuccess(), resp.GetMsg())
		}
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "jobs": resp.GetJobs(), "total": resp.GetTotal()})
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
	resp, err := h.svc.ListArtifacts(ctx, &conversation_intelligence.ListArtifactsReq{
		ViewerId:       viewerID,
		ConversationId: parseInt64Default(c.DefaultQuery("conversation_id", "0"), 0),
		ArtifactType:   strings.TrimSpace(c.DefaultQuery("type", "")),
		Limit:          parseInt64Default(c.DefaultQuery("limit", "20"), 20),
		Offset:         parseInt64Default(c.DefaultQuery("offset", "0"), 0),
	})
	if err != nil || !resp.GetSuccess() {
		if err == nil {
			err = conversationIntelStatusError(resp.GetSuccess(), resp.GetMsg())
		}
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "artifacts": resp.GetArtifacts(), "total": resp.GetTotal()})
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

type conversationMissedSummaryRequest struct {
	ConversationID json.Number `json:"conversation_id"`
	Limit          int         `json:"limit"`
}

func unreadMessagesFromHistory(messages []*message.Message) []*message.Message {
	out := make([]*message.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || msg.GetId() <= 0 || msg.GetIsReadByMe() {
			continue
		}
		out = append(out, msg)
	}
	return out
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

func conversationIntelStatusError(success bool, msg string) error {
	if success {
		return nil
	}
	if strings.TrimSpace(msg) == "" {
		msg = "conversation-intelligence-service RPC调用失败"
	}
	return errors.New(msg)
}
