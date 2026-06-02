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

// gatewayMemoryService 是 api-gateway 到 memory-service 的内部客户端。
// 通过接口注入可以保持前端路由稳定，同时避免网关直接依赖 memory-service 内部实现。
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

// ensureService 检查 memory-service 客户端是否已注入。
// 未初始化时返回明确错误，避免记忆管理页面触发 nil pointer panic。
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
	if err != nil || botID < 0 {
		response.BadRequest(c, "无效的记忆归属")
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
		Importance:     req.Importance,
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
	var importance *float64
	if req.Importance > 0 {
		importance = &req.Importance
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
		Importance:   importance,
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

// ListCandidates 返回等待用户确认的候选记忆。
func (h *MemoryHandler) ListCandidates(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	candidates, total, err := h.svc.ListCandidates(ctx, userID, memoryclient.CandidateFilter{
		BotID:  parseInt64Query(c, "bot_id"),
		UserID: parseInt64Query(c, "user_id"),
		Status: strings.TrimSpace(c.DefaultQuery("status", "pending")),
		Limit:  int(parseInt64Default(c.DefaultQuery("limit", "20"), 20)),
		Offset: int(parseInt64Default(c.DefaultQuery("offset", "0"), 0)),
	})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "candidates": candidates, "total": total})
}

// CreateCandidate 允许调试或前端把抽取结果先写入 pending 候选区。
func (h *MemoryHandler) CreateCandidate(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req memoryCandidateRequest
	if err := bindMemoryJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	botID, err := parseOptionalMemoryNumber(req.BotID)
	if err != nil || botID < 0 {
		response.BadRequest(c, "无效的记忆归属")
		return
	}
	targetUserID := parseMemoryNumberOrZero(req.UserID)
	if targetUserID == 0 {
		targetUserID = userID
	}
	if targetUserID != userID {
		response.Forbidden(c, "只能创建自己的候选记忆")
		return
	}
	candidate, err := h.svc.CreateCandidate(ctx, memoryclient.CandidateInput{
		BotID:              botID,
		UserID:             targetUserID,
		OwnerUserID:        userID,
		GroupID:            parseMemoryNumberOrZero(req.GroupID),
		ConversationID:     parseMemoryNumberOrZero(req.ConversationID),
		SessionID:          strings.TrimSpace(req.SessionID),
		Scope:              defaultMemoryScope(req.Scope),
		Type:               defaultMemoryType(req.Type),
		Title:              strings.TrimSpace(req.Title),
		Content:            strings.TrimSpace(req.Content),
		Source:             defaultMemorySource(req.Source),
		Evidence:           strings.TrimSpace(req.Evidence),
		Confidence:         req.Confidence,
		Importance:         req.Importance,
		ConflictMemoryIDs:  req.ConflictMemoryIDs,
		ConflictResolution: strings.TrimSpace(req.ConflictResolution),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "candidate": candidate})
}

// AcceptCandidate 将 pending 候选转成正式记忆。
func (h *MemoryHandler) AcceptCandidate(ctx context.Context, c *app.RequestContext) {
	h.handleCandidateAction(ctx, c, true)
}

// RejectCandidate 拒绝 pending 候选。
func (h *MemoryHandler) RejectCandidate(ctx context.Context, c *app.RequestContext) {
	h.handleCandidateAction(ctx, c, false)
}

func (h *MemoryHandler) handleCandidateAction(ctx context.Context, c *app.RequestContext, accept bool) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	candidateID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || candidateID <= 0 {
		response.BadRequest(c, "无效的候选记忆ID")
		return
	}
	var candidate *memoryclient.MemoryCandidate
	if accept {
		candidate, err = h.svc.AcceptCandidate(ctx, userID, candidateID)
	} else {
		candidate, err = h.svc.RejectCandidate(ctx, userID, candidateID)
	}
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "candidate": candidate})
}

// memoryRequest 是前端创建/编辑记忆事实的请求体。
// ID 字段使用 json.Number，避免雪花 ID 在浏览器和 Go 解码之间损失精度。
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
	Importance     float64     `json:"importance"`
}

type memoryCandidateRequest struct {
	BotID              json.Number `json:"bot_id"`
	UserID             json.Number `json:"user_id"`
	GroupID            json.Number `json:"group_id"`
	ConversationID     json.Number `json:"conversation_id"`
	SessionID          string      `json:"session_id"`
	Scope              string      `json:"scope"`
	Type               string      `json:"type"`
	Title              string      `json:"title"`
	Content            string      `json:"content"`
	Source             string      `json:"source"`
	Evidence           string      `json:"evidence"`
	Confidence         float64     `json:"confidence"`
	Importance         float64     `json:"importance"`
	ConflictMemoryIDs  []int64     `json:"conflict_memory_ids"`
	ConflictResolution string      `json:"conflict_resolution"`
}

// bindMemoryJSON 使用 UseNumber 解码，保护 bot_id、group_id 等大整数 ID。
func bindMemoryJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

// parseOptionalMemoryNumber 解析可选 JSON 数字字段，空值表示未指定。
func parseOptionalMemoryNumber(value json.Number) (int64, error) {
	if strings.TrimSpace(value.String()) == "" {
		return 0, nil
	}
	return strconv.ParseInt(value.String(), 10, 64)
}

// parseMemoryNumberOrZero 用于可选关联 ID，解析失败时按 0 处理交给下游默认逻辑。
func parseMemoryNumberOrZero(value json.Number) int64 {
	id, _ := parseOptionalMemoryNumber(value)
	return id
}

// parseInt64Query 读取查询参数中的 int64，缺省或非法时返回 0。
func parseInt64Query(c *app.RequestContext, key string) int64 {
	return parseInt64Default(c.DefaultQuery(key, "0"), 0)
}

// parseInt64Default 解析字符串整数，失败时返回调用方提供的 fallback。
func parseInt64Default(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

// parseCSVQuery 将逗号分隔查询参数转成去空白后的字符串列表。
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

// defaultMemoryScope 为空时默认创建个人记忆。
func defaultMemoryScope(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return memoryclient.ScopeUser
	}
	return strings.TrimSpace(scope)
}

// defaultMemoryType 为空时默认作为偏好类事实保存。
func defaultMemoryType(memoryType string) string {
	if strings.TrimSpace(memoryType) == "" {
		return memoryclient.TypePreference
	}
	return strings.TrimSpace(memoryType)
}

// defaultMemoryVisibility 为空时默认仅本人可见，避免误把个人画像共享给群聊。
func defaultMemoryVisibility(visibility string) string {
	if strings.TrimSpace(visibility) == "" {
		return memoryclient.VisibilityPrivate
	}
	return strings.TrimSpace(visibility)
}

// defaultMemoryVectorStatus 标记记忆尚未向量化；当前 MVP 可先不接向量库。
func defaultMemoryVectorStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return memoryclient.VectorPending
	}
	return strings.TrimSpace(status)
}

// defaultMemorySource 标明记忆由用户手动创建，区别于 Agent 总结或系统导入。
func defaultMemorySource(source string) string {
	if strings.TrimSpace(source) == "" {
		return "user_manual"
	}
	return strings.TrimSpace(source)
}
