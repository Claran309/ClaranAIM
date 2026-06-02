// Package handler 中的 AdminHandler 负责把管理后台 HTTP 请求转成 admin-service RPC。
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/admin"
	"ClaranAIM/pkg/response"
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

type AdminHandler struct{}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{}
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

func writeAdminResp(c *app.RequestContext, data interface{}, err error) {
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, data)
}
