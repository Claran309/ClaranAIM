// Package handler 中的 AdminHandler 负责把管理后台 HTTP 请求转成 admin-service RPC。
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/admin"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/response"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

type AdminHandler struct{}

var adminObservabilityConfig config.ObservabilityConfig

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{}
}

func InitAdminObservabilityLinks(cfg config.ObservabilityConfig) {
	adminObservabilityConfig = cfg
}

func (h *AdminHandler) Dashboard(ctx context.Context, c *app.RequestContext) {
	adminID := currentAdminID(c)
	resp, err := client.AdminClient.GetDashboard(ctx, &admin.DashboardReq{AdminId: adminID})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListUsers(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListUsers(ctx, &admin.ListUsersReq{
		AdminId:       currentAdminID(c),
		Keyword:       c.Query("keyword"),
		Role:          c.Query("role"),
		Status:        c.Query("status"),
		IncludeSystem: queryBool(c, "include_system"),
		Limit:         queryInt(c, "limit", 50),
		Offset:        queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) UpdateUserStatus(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID := pathOrQueryInt(c, "id", "user_id", 0)
	if userID <= 0 {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	status := strings.TrimSpace(req.Status)
	if status != "banned" && status != "offline" && status != "online" {
		response.BadRequest(c, "status只能是banned、offline或online")
		return
	}
	resp, err := client.UserClient.UpdateStatus(ctx, &user.UpdateStatusReq{UserId: userID, Status: status})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *AdminHandler) ListGroups(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListGroups(ctx, &admin.ListGroupsReq{
		AdminId: currentAdminID(c),
		Keyword: c.Query("keyword"),
		OwnerId: queryInt(c, "owner_id", 0),
		Limit:   queryInt(c, "limit", 50),
		Offset:  queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) UpdateGroupStatus(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID := pathOrQueryInt(c, "id", "group_id", 0)
	if groupID <= 0 {
		response.BadRequest(c, "无效的群ID")
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != "active" && status != "banned" {
		response.BadRequest(c, "status只能是active或banned")
		return
	}
	resp, err := client.AdminClient.UpdateGroupStatus(ctx, &admin.UpdateGroupStatusReq{
		AdminId: currentAdminID(c),
		GroupId: groupID,
		Status:  status,
		Reason:  req.Reason,
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListFiles(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListFiles(ctx, &admin.ListFilesReq{
		AdminId:    currentAdminID(c),
		UploaderId: queryInt(c, "uploader_id", 0),
		FileType:   c.Query("file_type"),
		Limit:      queryInt(c, "limit", 50),
		Offset:     queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListAgents(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListAgents(ctx, &admin.ListAgentsReq{
		AdminId: currentAdminID(c),
		OwnerId: queryInt(c, "owner_id", 0),
		Type:    c.Query("type"),
		Limit:   queryInt(c, "limit", 50),
		Offset:  queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListBilling(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListBilling(ctx, &admin.ListBillingReq{
		AdminId: currentAdminID(c),
		BotId:   queryInt(c, "bot_id", 0),
		UserId:  queryInt(c, "user_id", 0),
		Limit:   queryInt(c, "limit", 50),
		Offset:  queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListReviews(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListReviews(ctx, &admin.ListReviewsReq{
		AdminId: currentAdminID(c),
		Source:  c.Query("source"),
		Status:  c.Query("status"),
		Limit:   queryInt(c, "limit", 50),
		Offset:  queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ReviewItem(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Source string `json:"source"`
		ItemID int64  `json:"item_id"`
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	resp, err := client.AdminClient.ReviewItem(ctx, &admin.ReviewReq{AdminId: currentAdminID(c), Source: req.Source, ItemId: req.ItemID, Action: req.Action, Note: req.Note})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListMCPTraces(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListMCPTraces(ctx, &admin.ListMCPTracesReq{
		AdminId:        currentAdminID(c),
		AgentId:        queryInt(c, "agent_id", 0),
		ConversationId: queryInt(c, "conversation_id", 0),
		Limit:          queryInt(c, "limit", 50),
		Offset:         queryInt(c, "offset", 0),
	})
	if err == nil && strings.EqualFold(strings.TrimSpace(c.Query("group_by")), "trace_id") && resp != nil && resp.Success {
		response.Success(c, groupAdminMCPTraces(resp))
		return
	}
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) SaveNotice(ctx context.Context, c *app.RequestContext) {
	var req struct {
		NoticeID int64  `json:"notice_id"`
		Title    string `json:"title"`
		Content  string `json:"content"`
		Level    string `json:"level"`
		Audience string `json:"audience"`
		Enabled  bool   `json:"enabled"`
	}
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	resp, err := client.AdminClient.SaveNotice(ctx, &admin.SaveNoticeReq{AdminId: currentAdminID(c), NoticeId: req.NoticeID, Title: req.Title, Content: req.Content, Level: req.Level, Audience: req.Audience, Enabled: req.Enabled})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListNotices(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListNotices(ctx, &admin.ListNoticesReq{
		AdminId:         currentAdminID(c),
		IncludeDisabled: queryBool(c, "include_disabled"),
		Limit:           queryInt(c, "limit", 50),
		Offset:          queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListPublicNotices(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListNotices(ctx, &admin.ListNoticesReq{
		AdminId:         0,
		IncludeDisabled: false,
		Limit:           queryInt(c, "limit", 20),
		Offset:          queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ListAuditLogs(ctx context.Context, c *app.RequestContext) {
	resp, err := client.AdminClient.ListAuditLogs(ctx, &admin.ListAuditLogsReq{
		AdminId:    currentAdminID(c),
		Action:     c.Query("action"),
		TargetType: c.Query("target_type"),
		Limit:      queryInt(c, "limit", 50),
		Offset:     queryInt(c, "offset", 0),
	})
	writeAdminResp(c, resp, err)
}

func (h *AdminHandler) ObservabilityLinks(ctx context.Context, c *app.RequestContext) {
	cfg := adminObservabilityConfig
	environment := strings.TrimSpace(cfg.Environment)
	if environment == "" {
		environment = "dev"
	}
	links := []map[string]string{
		{
			"key":         "grafana",
			"title":       "Grafana 总览",
			"description": "统一查看服务 QPS、延迟、错误率、依赖检查和业务指标；No Data 通常表示 Prometheus target 未 up。",
			"url":         firstConfiguredURL(cfg.GrafanaURL, cfg.DashboardBaseURL, "http://127.0.0.1:8086"),
		},
		{
			"key":         "jaeger",
			"title":       "Jaeger Trace",
			"description": "查看 api-gateway 到内部 Kitex 服务的调用链。",
			"url":         firstConfiguredURL(cfg.JaegerURL, "http://127.0.0.1:8085"),
		},
		{
			"key":         "kibana",
			"title":       "Kibana 日志",
			"description": "检索 logs/ 下由 Filebeat 汇入 ELK 的结构化服务日志；Data View 使用 claran-services-*，时间字段 @timestamp。",
			"url":         firstConfiguredURL(cfg.KibanaURL, "http://127.0.0.1:5601"),
		},
		{
			"key":         "prometheus",
			"title":       "Prometheus Targets",
			"description": "检查各服务 metrics 端口是否被 Prometheus 成功抓取。",
			"url":         strings.TrimRight(firstConfiguredURL(cfg.PrometheusURL, "http://127.0.0.1:8084"), "/") + "/targets",
		},
	}
	response.Success(c, map[string]interface{}{
		"success":         true,
		"enabled":         cfg.Enabled,
		"environment":     environment,
		"otlp_endpoint":   cfg.OTLPEndpoint,
		"metrics_address": cfg.MetricsAddress,
		"hints": []string{
			"Prometheus Targets 先确认 claran-go-services 是否 up；如果 host.docker.internal 解析失败，检查 prometheus compose 的 extra_hosts。",
			"Grafana No Data 通常不是前端问题，而是 Prometheus 没抓到对应 metrics。",
			"Kibana 首次使用需要创建 Data View：claran-services-*，时间字段 @timestamp。",
			"Jaeger 只有服务发出 OTLP trace 后才会出现调用链。",
		},
		"links": links,
	})
}

func currentAdminID(c *app.RequestContext) int64 {
	id, _ := c.Get("userID")
	if v, ok := id.(int64); ok {
		return v
	}
	return 0
}

func queryInt(c *app.RequestContext, key string, fallback int64) int64 {
	raw := c.Query(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func queryBool(c *app.RequestContext, key string) bool {
	value, _ := strconv.ParseBool(c.Query(key))
	return value
}

func pathOrQueryInt(c *app.RequestContext, pathKey, queryKey string, fallback int64) int64 {
	raw := c.Param(pathKey)
	if raw == "" {
		raw = c.Query(queryKey)
	}
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func writeAdminResp(c *app.RequestContext, data interface{}, err error) {
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, data)
}

func groupAdminMCPTraces(resp *admin.ListMCPTracesResp) map[string]interface{} {
	groups := make([]map[string]interface{}, 0)
	index := map[string]int{}
	for _, trace := range resp.GetTraces() {
		if trace == nil {
			continue
		}
		key := strings.TrimSpace(trace.TraceId)
		if key == "" {
			key = fmt.Sprintf("trace:%d", trace.Id)
		}
		pos, ok := index[key]
		if !ok {
			pos = len(groups)
			index[key] = pos
			groups = append(groups, map[string]interface{}{
				"id":               trace.Id,
				"trace_id":         trace.TraceId,
				"user_id":          trace.UserId,
				"agent_id":         trace.AgentId,
				"conversation_id":  trace.ConversationId,
				"tool_name":        trace.ToolName,
				"source":           trace.Source,
				"server_name":      trace.ServerName,
				"status":           trace.Status,
				"latency_ms":       trace.LatencyMs,
				"error_message":    trace.ErrorMessage,
				"created_at":       trace.CreatedAt,
				"call_count":       int64(0),
				"total_latency_ms": int64(0),
				"tools":            []string{},
				"calls":            []*admin.AdminMCPTrace{},
			})
		}
		group := groups[pos]
		group["call_count"] = group["call_count"].(int64) + 1
		group["total_latency_ms"] = group["total_latency_ms"].(int64) + trace.LatencyMs
		if strings.EqualFold(trace.Status, "error") || strings.EqualFold(trace.Status, "failed") {
			group["status"] = trace.Status
			group["error_message"] = trace.ErrorMessage
		}
		tools := group["tools"].([]string)
		if trace.ToolName != "" && !stringSliceContains(tools, trace.ToolName) {
			group["tools"] = append(tools, trace.ToolName)
		}
		group["calls"] = append(group["calls"].([]*admin.AdminMCPTrace), trace)
	}
	return map[string]interface{}{
		"success": true,
		"traces":  groups,
		"total":   resp.GetTotal(),
		"msg":     resp.GetMsg(),
	}
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstConfiguredURL(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
