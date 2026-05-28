package handler

import (
	"ClaranAIM/pkg/memoryclient"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// MemoryHandler 通过 api-gateway 暴露用户可管理的 Agent 记忆接口。
type MemoryHandler struct {
	svc memoryclient.Service
}

// 下面这组变量保存当前包需要复用的运行时状态或配置入口，调用方应通过公开函数间接使用。
var gatewayMemoryService memoryclient.Service

// InitMemoryService 注册当前进程内的 memory-service 门面。
//
// 后续如果 memory-service 拆成独立 Kitex 服务，浏览器侧路由可以保持不变，
// 只需要替换这里注入的客户端实现。
func InitMemoryService(svc memoryclient.Service) {
	gatewayMemoryService = svc
}

// NewMemoryHandler 创建记忆管理 HTTP handler。
func NewMemoryHandler() *MemoryHandler {
	return &MemoryHandler{svc: gatewayMemoryService}
}

// ensureService 是当前包内部使用的方法，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func (h *MemoryHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "memory-service未初始化")
		return false
	}
	return true
}

// ListMemories 返回当前用户可查看和编辑的记忆事实列表。
func (h *MemoryHandler) ListMemories(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	filter := memoryclient.Filter{
		BotID:           parseInt64Query(c, "bot_id"),
		UserID:          parseInt64Query(c, "user_id"),
		GroupID:         parseInt64Query(c, "group_id"),
		ConversationID:  parseInt64Query(c, "conversation_id"),
		SessionID:       strings.TrimSpace(c.DefaultQuery("session_id", "")),
		IncludeDisabled: c.DefaultQuery("include_disabled", "false") == "true",
		Limit:           int(parseInt64Default(c.DefaultQuery("limit", "20"), 20)),
		Offset:          int(parseInt64Default(c.DefaultQuery("offset", "0"), 0)),
	}
	if filter.UserID == 0 {
		filter.UserID = userID
	}
	if scopes := parseCSVQuery(c.DefaultQuery("scope", "")); len(scopes) > 0 {
		filter.Scopes = scopes
	}
	if types := parseCSVQuery(c.DefaultQuery("type", "")); len(types) > 0 {
		filter.Types = types
	}
	facts, total, err := h.svc.ListMemories(ctx, userID, filter)
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{
		"success":  true,
		"memories": facts,
		"total":    total,
	})
}

// CreateMemory 允许用户显式新增个人、会话或群聊范围的记忆事实。
func (h *MemoryHandler) CreateMemory(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req memoryRequest
	if err := bindMemoryJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseOptionalMemoryNumber(req.BotID)
	if err != nil || botID <= 0 {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	targetUserID, err := parseOptionalMemoryNumber(req.UserID)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	if targetUserID == 0 {
		targetUserID = userID
	}
	if targetUserID != userID {
		response.Forbidden(c, "只能创建自己的记忆")
		return
	}
	input := memoryclient.CreateMemoryInput{
		BotID:          botID,
		UserID:         targetUserID,
		OwnerUserID:    userID,
		GroupID:        parseMemoryNumberOrZero(req.GroupID),
		ConversationID: parseMemoryNumberOrZero(req.ConversationID),
		SessionID:      strings.TrimSpace(req.SessionID),
		Scope:          defaultMemoryScope(req.Scope),
		Type:           defaultMemoryType(req.Type),
		Title:          req.Title,
		Content:        req.Content,
		Source:         defaultMemorySource(req.Source),
		Visibility:     defaultMemoryVisibility(req.Visibility),
		Enabled:        req.Enabled,
		VectorStatus:   defaultMemoryVectorStatus(req.VectorStatus),
		Confidence:     req.Confidence,
	}
	fact, err := h.svc.CreateMemory(ctx, input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "memory": fact})
}

// UpdateMemory 编辑或禁用当前用户拥有的一条记忆事实。
func (h *MemoryHandler) UpdateMemory(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	memoryID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || memoryID <= 0 {
		response.BadRequest(c, "无效的记忆ID")
		return
	}
	var req memoryRequest
	if err := bindMemoryJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	var confidence *float64
	if req.Confidence > 0 {
		confidence = &req.Confidence
	}
	fact, err := h.svc.UpdateMemory(ctx, userID, memoryID, memoryclient.UpdateMemoryInput{
		Scope:        req.Scope,
		Type:         req.Type,
		Title:        req.Title,
		Content:      req.Content,
		Source:       req.Source,
		Visibility:   req.Visibility,
		Enabled:      req.Enabled,
		VectorStatus: req.VectorStatus,
		Confidence:   confidence,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "memory": fact})
}

// DeleteMemory 删除当前用户拥有的一条记忆事实。
func (h *MemoryHandler) DeleteMemory(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	memoryID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || memoryID <= 0 {
		response.BadRequest(c, "无效的记忆ID")
		return
	}
	if err := h.svc.DeleteMemory(ctx, userID, memoryID); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true})
}

// memoryRequest 定义当前包使用的数据结构或接口，用于在业务层、持久化层和传输层之间传递明确语义。
type memoryRequest struct {
	BotID          json.Number `json:"bot_id"`
	UserID         json.Number `json:"user_id"`
	GroupID        json.Number `json:"group_id"`
	ConversationID json.Number `json:"conversation_id"`
	SessionID      string      `json:"session_id"`
	Scope          string      `json:"scope"`
	Type           string      `json:"type"`
	Title          string      `json:"title"`
	Content        string      `json:"content"`
	Source         string      `json:"source"`
	Visibility     string      `json:"visibility"`
	Enabled        *bool       `json:"enabled"`
	VectorStatus   string      `json:"vector_status"`
	Confidence     float64     `json:"confidence"`
}

// bindMemoryJSON 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func bindMemoryJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

// parseOptionalMemoryNumber 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func parseOptionalMemoryNumber(value json.Number) (int64, error) {
	if strings.TrimSpace(value.String()) == "" {
		return 0, nil
	}
	return strconv.ParseInt(value.String(), 10, 64)
}

// parseMemoryNumberOrZero 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func parseMemoryNumberOrZero(value json.Number) int64 {
	id, _ := parseOptionalMemoryNumber(value)
	return id
}

// parseInt64Query 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func parseInt64Query(c *app.RequestContext, key string) int64 {
	return parseInt64Default(c.DefaultQuery(key, "0"), 0)
}

// parseInt64Default 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func parseInt64Default(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

// parseCSVQuery 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func parseCSVQuery(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
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

// defaultMemoryScope 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func defaultMemoryScope(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return memoryclient.ScopeUser
	}
	return strings.TrimSpace(scope)
}

// defaultMemoryType 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func defaultMemoryType(memoryType string) string {
	if strings.TrimSpace(memoryType) == "" {
		return memoryclient.TypePreference
	}
	return strings.TrimSpace(memoryType)
}

// defaultMemoryVisibility 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func defaultMemoryVisibility(visibility string) string {
	if strings.TrimSpace(visibility) == "" {
		return memoryclient.VisibilityPrivate
	}
	return strings.TrimSpace(visibility)
}

// defaultMemoryVectorStatus 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func defaultMemoryVectorStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return memoryclient.VectorPending
	}
	return strings.TrimSpace(status)
}

// defaultMemorySource 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func defaultMemorySource(source string) string {
	if strings.TrimSpace(source) == "" {
		return "user_manual"
	}
	return strings.TrimSpace(source)
}
