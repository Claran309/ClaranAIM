package handler

import (
	"ClaranAIM/pkg/knowledgeclient"
	"ClaranAIM/pkg/response"
	"context"
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

func parseKnowledgeGraphQuery(c *app.RequestContext) knowledgeclient.GraphQuery {
	return knowledgeclient.GraphQuery{
		Query:           strings.TrimSpace(c.DefaultQuery("query", "")),
		TypeFilters:     parseKnowledgeCSV(c.DefaultQuery("types", "")),
		RelationFilters: parseKnowledgeCSV(c.DefaultQuery("relations", "")),
		CommunityID:     parseKnowledgeInt64(c.DefaultQuery("community_id", "0"), 0),
		Hops:            int(parseKnowledgeInt64(c.DefaultQuery("hops", "1"), 1)),
		Limit:           int(parseKnowledgeInt64(c.DefaultQuery("limit", "160"), 160)),
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
