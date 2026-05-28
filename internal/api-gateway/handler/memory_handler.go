package handler

import (
	"ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/model"
	memorysvc "ClaranAIM/internal/memory-service/service"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// MemoryHandler exposes user-governed Agent memory APIs through api-gateway.
type MemoryHandler struct {
	svc memorysvc.MemoryService
}

var gatewayMemoryService memorysvc.MemoryService

// InitMemoryService wires the local memory service facade. The implementation
// can later move behind Kitex without changing browser-facing routes.
func InitMemoryService(svc memorysvc.MemoryService) {
	gatewayMemoryService = svc
}

// NewMemoryHandler creates the memory HTTP handler.
func NewMemoryHandler() *MemoryHandler {
	return &MemoryHandler{svc: gatewayMemoryService}
}

func (h *MemoryHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "memory-service未初始化")
		return false
	}
	return true
}

// ListMemories returns the current user's editable memory facts.
func (h *MemoryHandler) ListMemories(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	filter := dao.MemoryFilter{
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

// CreateMemory lets the user explicitly add a profile, conversation or group memory.
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
		response.BadRequest(c, "无效的Bot ID")
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
	input := memorysvc.CreateMemoryInput{
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

// UpdateMemory edits or disables one owned memory fact.
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
	fact, err := h.svc.UpdateMemory(ctx, userID, memoryID, memorysvc.UpdateMemoryInput{
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

// DeleteMemory removes one owned memory fact.
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

func bindMemoryJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

func parseOptionalMemoryNumber(value json.Number) (int64, error) {
	if strings.TrimSpace(value.String()) == "" {
		return 0, nil
	}
	return strconv.ParseInt(value.String(), 10, 64)
}

func parseMemoryNumberOrZero(value json.Number) int64 {
	id, _ := parseOptionalMemoryNumber(value)
	return id
}

func parseInt64Query(c *app.RequestContext, key string) int64 {
	return parseInt64Default(c.DefaultQuery(key, "0"), 0)
}

func parseInt64Default(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

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

func defaultMemoryScope(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return model.ScopeUser
	}
	return strings.TrimSpace(scope)
}

func defaultMemoryType(memoryType string) string {
	if strings.TrimSpace(memoryType) == "" {
		return model.TypePreference
	}
	return strings.TrimSpace(memoryType)
}

func defaultMemoryVisibility(visibility string) string {
	if strings.TrimSpace(visibility) == "" {
		return model.VisibilityPrivate
	}
	return strings.TrimSpace(visibility)
}

func defaultMemoryVectorStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return model.VectorPending
	}
	return strings.TrimSpace(status)
}

func defaultMemorySource(source string) string {
	if strings.TrimSpace(source) == "" {
		return "user_manual"
	}
	return strings.TrimSpace(source)
}
