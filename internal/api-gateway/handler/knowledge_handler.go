package handler

import (
	"ClaranAIM/pkg/knowledgeclient"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// KnowledgeHandler 暴露知识图谱可视化查询接口。
// 它只处理 HTTP 参数、登录态和响应格式，图谱过滤、详情聚合等逻辑由 knowledgeclient.Service 完成。
type KnowledgeHandler struct {
	svc knowledgeclient.Service
}

var gatewayKnowledgeService knowledgeclient.Service

// InitKnowledgeService 注册知识图谱视图服务。
func InitKnowledgeService(svc knowledgeclient.Service) {
	gatewayKnowledgeService = svc
}

// NewKnowledgeHandler 创建知识图谱 HTTP handler。
func NewKnowledgeHandler() *KnowledgeHandler {
	return &KnowledgeHandler{svc: gatewayKnowledgeService}
}

func (h *KnowledgeHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "knowledge-service未初始化")
		return false
	}
	return true
}

// GetGraphView 返回前端画布需要的节点、边、社区和过滤 facet。
func (h *KnowledgeHandler) GetGraphView(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	view, err := h.svc.GetGraphView(ctx, userID, parseKnowledgeGraphQuery(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, view)
}

// GetNodeDetail 返回节点详情、相邻节点和相关关系。
func (h *KnowledgeHandler) GetNodeDetail(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	nodeID := parseKnowledgeInt64(c.Param("id"), 0)
	if nodeID <= 0 {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	detail, err := h.svc.GetNodeDetail(ctx, userID, nodeID, parseKnowledgeGraphQuery(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, detail)
}

// GetEdgeDetail 返回关系详情和两端节点。
func (h *KnowledgeHandler) GetEdgeDetail(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	edgeID := parseKnowledgeInt64(c.Param("id"), 0)
	if edgeID <= 0 {
		response.BadRequest(c, "无效的关系ID")
		return
	}
	detail, err := h.svc.GetEdgeDetail(ctx, userID, edgeID, parseKnowledgeGraphQuery(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, detail)
}

// GetNeighborhood 返回指定节点的一跳或多跳邻域子图。
func (h *KnowledgeHandler) GetNeighborhood(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	nodeID := parseKnowledgeInt64(c.Param("id"), 0)
	if nodeID <= 0 {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	view, err := h.svc.GetNeighborhood(ctx, userID, nodeID, parseKnowledgeGraphQuery(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, view)
}

// GetPath 返回两个节点之间的最短可见路径。
func (h *KnowledgeHandler) GetPath(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	sourceID := parseKnowledgeInt64(c.DefaultQuery("source_id", "0"), 0)
	targetID := parseKnowledgeInt64(c.DefaultQuery("target_id", "0"), 0)
	if sourceID <= 0 || targetID <= 0 {
		response.BadRequest(c, "source_id和target_id不能为空")
		return
	}
	path, err := h.svc.GetPath(ctx, userID, sourceID, targetID, parseKnowledgeGraphQuery(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, path)
}

// CreateReviewCandidate 将当前图谱节点或关系提交到审核工作台。
func (h *KnowledgeHandler) CreateReviewCandidate(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req struct {
		ItemType string      `json:"item_type"`
		ItemID   json.Number `json:"item_id"`
		Reason   string      `json:"reason"`
		Query    string      `json:"query"`
	}
	if err := bindJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	itemID, err := numberToInt64(req.ItemID)
	if err != nil || itemID <= 0 {
		response.BadRequest(c, "无效的候选对象ID")
		return
	}
	candidate, err := h.svc.CreateGraphReviewCandidate(ctx, userID, knowledgeclient.CreateGraphReviewCandidateInput{
		ItemType: req.ItemType,
		ItemID:   itemID,
		Reason:   req.Reason,
		Query:    req.Query,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "candidate": candidate})
}

// ListReviewCandidates 返回当前用户的图谱审核候选。
func (h *KnowledgeHandler) ListReviewCandidates(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	list, err := h.svc.ListGraphReviewCandidates(ctx, userID, knowledgeclient.ListGraphReviewCandidatesInput{
		Status:   strings.TrimSpace(c.DefaultQuery("status", "")),
		ItemType: strings.TrimSpace(c.DefaultQuery("item_type", "")),
		Limit:    int(parseKnowledgeInt64(c.DefaultQuery("limit", "50"), 50)),
		Offset:   int(parseKnowledgeInt64(c.DefaultQuery("offset", "0"), 0)),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, list)
}

// ReviewCandidate 对图谱候选执行通过或拒绝。
func (h *KnowledgeHandler) ReviewCandidate(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	candidateID := parseKnowledgeInt64(c.Param("id"), 0)
	if candidateID <= 0 {
		response.BadRequest(c, "无效的候选ID")
		return
	}
	var req struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if err := bindJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	candidate, err := h.svc.ReviewGraphCandidate(ctx, userID, knowledgeclient.ReviewGraphCandidateInput{
		CandidateID: candidateID,
		Action:      req.Action,
		Note:        req.Note,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "candidate": candidate})
}

func parseKnowledgeGraphQuery(c *app.RequestContext) knowledgeclient.GraphQuery {
	return knowledgeclient.GraphQuery{
		Query:           strings.TrimSpace(c.DefaultQuery("query", "")),
		TypeFilters:     parseKnowledgeCSV(c.DefaultQuery("types", "")),
		RelationFilters: parseKnowledgeCSV(c.DefaultQuery("relations", "")),
		CommunityID:     parseKnowledgeInt64(c.DefaultQuery("community_id", "0"), 0),
		Hops:            int(parseKnowledgeInt64(c.DefaultQuery("hops", "1"), 1)),
		Limit:           int(parseKnowledgeInt64(c.DefaultQuery("limit", "160"), 160)),
		DocumentID:      parseKnowledgeInt64(c.DefaultQuery("document_id", "0"), 0),
	}
}

func parseKnowledgeCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseKnowledgeInt64(value string, fallback int64) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
