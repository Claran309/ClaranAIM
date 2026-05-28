// Package service 实现可由用户治理的 Agent 记忆事实和召回规则。
package service

import (
	"ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/model"
	"ClaranAIM/pkg/memoryclient"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MemoryService 暴露 Phase4 记忆能力，供 api-gateway 和 Agent runtime 调用。
type MemoryService interface {
	CreateMemory(ctx context.Context, input CreateMemoryInput) (*memoryclient.MemoryFact, error)
	ListMemories(ctx context.Context, viewerID int64, filter dao.MemoryFilter) ([]memoryclient.MemoryFact, int64, error)
	Recall(ctx context.Context, input RecallInput) (RecallResult, error)
	UpdateMemory(ctx context.Context, viewerID, memoryID int64, input UpdateMemoryInput) (*memoryclient.MemoryFact, error)
	DeleteMemory(ctx context.Context, viewerID, memoryID int64) error
}

// CreateMemoryInput 是创建单条记忆事实的 API 入参结构。
type CreateMemoryInput = memoryclient.CreateMemoryInput

// UpdateMemoryInput 保存用户可编辑记忆的可选更新字段。
type UpdateMemoryInput = memoryclient.UpdateMemoryInput

// RecallInput 限定记忆召回边界，避免把不相关用户、群或会话的记忆注入 Agent。
type RecallInput = memoryclient.RecallInput

// RecallResult 保存召回到的记忆事实，以及可直接拼入 prompt 的文本块。
type RecallResult = memoryclient.RecallResult

// memoryServiceImpl 是 MemoryService 的默认实现，负责参数校验、权限裁剪和 DTO 转换。
type memoryServiceImpl struct {
	repo dao.MemoryRepository
}

// NewMemoryService 创建记忆业务服务。
func NewMemoryService(repo dao.MemoryRepository) MemoryService {
	return &memoryServiceImpl{repo: repo}
}

// CreateMemory 校验并保存一条记忆事实。
// 用户偏好、发言习惯等个人画像默认私有，因为这些信息只应由本人查看和管理。
func (s *memoryServiceImpl) CreateMemory(ctx context.Context, input CreateMemoryInput) (*memoryclient.MemoryFact, error) {
	if s.repo == nil {
		return nil, errors.New("memory repository未配置")
	}
	if err := validateCreateInput(input); err != nil {
		return nil, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	ownerID := input.OwnerUserID
	if ownerID == 0 {
		ownerID = input.UserID
	}
	visibility := input.Visibility
	if visibility == "" {
		visibility = model.VisibilityPrivate
	}
	vectorStatus := input.VectorStatus
	if vectorStatus == "" {
		vectorStatus = model.VectorPending
	}
	fact := &model.MemoryFact{
		BotID:          input.BotID,
		UserID:         input.UserID,
		OwnerUserID:    ownerID,
		GroupID:        input.GroupID,
		ConversationID: input.ConversationID,
		SessionID:      strings.TrimSpace(input.SessionID),
		Scope:          input.Scope,
		Type:           input.Type,
		Title:          strings.TrimSpace(input.Title),
		Content:        strings.TrimSpace(input.Content),
		Source:         strings.TrimSpace(input.Source),
		Visibility:     visibility,
		Enabled:        enabled,
		VectorStatus:   vectorStatus,
		EmbeddingRef:   strings.TrimSpace(input.EmbeddingRef),
		Confidence:     input.Confidence,
	}
	if err := s.repo.Create(ctx, fact); err != nil {
		return nil, err
	}
	return memoryFactToDTO(fact), nil
}

// ListMemories 只返回当前用户拥有的记忆。
// 即使调用方传入更宽的过滤条件，这里也会强制按 owner_user_id 裁剪，避免个人画像被越权列出。
func (s *memoryServiceImpl) ListMemories(ctx context.Context, viewerID int64, filter dao.MemoryFilter) ([]memoryclient.MemoryFact, int64, error) {
	if s.repo == nil {
		return nil, 0, errors.New("memory repository未配置")
	}
	if viewerID <= 0 {
		return nil, 0, errors.New("用户未登录")
	}
	filter.OwnerUserID = viewerID
	facts, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return memoryFactsToDTO(facts), total, nil
}

// Recall 只召回匹配当前 Agent、用户和上下文边界的记忆。
// 用户级记忆可以跨会话使用；群、会话、session 级记忆必须精确匹配，避免记忆串线。
func (s *memoryServiceImpl) Recall(ctx context.Context, input RecallInput) (RecallResult, error) {
	if s.repo == nil {
		return RecallResult{}, errors.New("memory repository未配置")
	}
	if input.BotID <= 0 || input.UserID <= 0 {
		return RecallResult{}, errors.New("bot_id和user_id不能为空")
	}
	limit := input.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	candidates, _, err := s.repo.List(ctx, dao.MemoryFilter{
		BotID:           input.BotID,
		UserID:          input.UserID,
		IncludeDisabled: false,
		Limit:           100,
	})
	if err != nil {
		return RecallResult{}, err
	}
	facts := make([]model.MemoryFact, 0, minInt(limit, len(candidates)))
	for _, fact := range candidates {
		if recallMatchesContext(fact, input) {
			facts = append(facts, fact)
			if len(facts) >= limit {
				break
			}
		}
	}
	if len(facts) > 0 {
		ids := make([]int64, 0, len(facts))
		for _, fact := range facts {
			ids = append(ids, fact.ID)
		}
		_ = s.repo.Touch(ctx, ids, time.Now())
	}
	return RecallResult{Facts: memoryFactsToDTO(facts), ContextText: FormatMemoryContext(facts)}, nil
}

// UpdateMemory 修改当前用户拥有的记忆。
// 用户可以编辑、关闭或调整自己的记忆，但不能修改他人的记忆事实。
func (s *memoryServiceImpl) UpdateMemory(ctx context.Context, viewerID, memoryID int64, input UpdateMemoryInput) (*memoryclient.MemoryFact, error) {
	fact, err := s.loadOwnedMemory(ctx, viewerID, memoryID)
	if err != nil {
		return nil, err
	}
	if input.Scope != "" {
		fact.Scope = input.Scope
	}
	if input.Type != "" {
		fact.Type = input.Type
	}
	if input.Title != "" {
		fact.Title = strings.TrimSpace(input.Title)
	}
	if input.Content != "" {
		fact.Content = strings.TrimSpace(input.Content)
	}
	if input.Source != "" {
		fact.Source = strings.TrimSpace(input.Source)
	}
	if input.Visibility != "" {
		fact.Visibility = input.Visibility
	}
	if input.Enabled != nil {
		fact.Enabled = *input.Enabled
	}
	if input.VectorStatus != "" {
		fact.VectorStatus = input.VectorStatus
	}
	if input.EmbeddingRef != "" {
		fact.EmbeddingRef = strings.TrimSpace(input.EmbeddingRef)
	}
	if input.Confidence != nil {
		fact.Confidence = *input.Confidence
	}
	if err := validateFact(*fact); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, fact); err != nil {
		return nil, err
	}
	return memoryFactToDTO(fact), nil
}

// DeleteMemory 删除当前用户拥有的记忆事实。
func (s *memoryServiceImpl) DeleteMemory(ctx context.Context, viewerID, memoryID int64) error {
	_, err := s.loadOwnedMemory(ctx, viewerID, memoryID)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, memoryID)
}

// loadOwnedMemory 读取一条记忆并校验当前用户是否为 owner。
func (s *memoryServiceImpl) loadOwnedMemory(ctx context.Context, viewerID, memoryID int64) (*model.MemoryFact, error) {
	if s.repo == nil {
		return nil, errors.New("memory repository未配置")
	}
	if viewerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	if memoryID <= 0 {
		return nil, errors.New("memory_id不能为空")
	}
	fact, err := s.repo.GetByID(ctx, memoryID)
	if err != nil {
		return nil, err
	}
	if fact == nil {
		return nil, errors.New("记忆不存在")
	}
	if fact.OwnerUserID != viewerID {
		return nil, errors.New("只能管理自己的记忆")
	}
	return fact, nil
}

// validateCreateInput 校验创建记忆所需的必填字段和枚举值。
func validateCreateInput(input CreateMemoryInput) error {
	if input.BotID <= 0 {
		return errors.New("bot_id不能为空")
	}
	if input.UserID <= 0 {
		return errors.New("user_id不能为空")
	}
	fact := model.MemoryFact{
		Scope:        input.Scope,
		Type:         input.Type,
		Content:      strings.TrimSpace(input.Content),
		Visibility:   defaultString(input.Visibility, model.VisibilityPrivate),
		VectorStatus: defaultString(input.VectorStatus, model.VectorPending),
	}
	return validateFact(fact)
}

// validateFact 校验记忆事实的范围、类型、可见性、向量状态和内容。
func validateFact(fact model.MemoryFact) error {
	if !validScope(fact.Scope) {
		return fmt.Errorf("无效的记忆范围: %s", fact.Scope)
	}
	if !validType(fact.Type) {
		return fmt.Errorf("无效的记忆类型: %s", fact.Type)
	}
	if strings.TrimSpace(fact.Content) == "" {
		return errors.New("记忆内容不能为空")
	}
	if !validVisibility(fact.Visibility) {
		return fmt.Errorf("无效的记忆可见性: %s", fact.Visibility)
	}
	if !validVectorStatus(fact.VectorStatus) {
		return fmt.Errorf("无效的向量状态: %s", fact.VectorStatus)
	}
	return nil
}

// recallMatchesContext 判断一条记忆是否允许进入当前召回上下文。
func recallMatchesContext(fact model.MemoryFact, input RecallInput) bool {
	switch fact.Scope {
	case model.ScopeUser:
		return true
	case model.ScopeGroup:
		return input.GroupID > 0 && fact.GroupID == input.GroupID
	case model.ScopeConversation:
		return input.ConversationID > 0 && fact.ConversationID == input.ConversationID
	case model.ScopeSession:
		return input.SessionID != "" && fact.SessionID == input.SessionID
	default:
		return false
	}
}

// FormatMemoryContext 将召回到的记忆事实整理为紧凑的 prompt 文本块。
func FormatMemoryContext(facts []model.MemoryFact) string {
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("可用长期记忆：\n")
	for _, fact := range facts {
		label := fact.Type
		if fact.Scope != "" {
			label = fact.Type + "/" + fact.Scope
		}
		content := strings.TrimSpace(fact.Content)
		if fact.Title != "" {
			content = strings.TrimSpace(fact.Title) + "：" + content
		}
		b.WriteString("- [")
		b.WriteString(label)
		b.WriteString("] ")
		b.WriteString(content)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// FormatClientMemoryContext 将 DTO 记忆转换成 prompt 文本，供 memory-service 外部调用方使用。
func FormatClientMemoryContext(facts []memoryclient.MemoryFact) string {
	modelFacts := make([]model.MemoryFact, 0, len(facts))
	for _, fact := range facts {
		modelFacts = append(modelFacts, model.MemoryFact{
			Scope:   fact.Scope,
			Type:    fact.Type,
			Title:   fact.Title,
			Content: fact.Content,
		})
	}
	return FormatMemoryContext(modelFacts)
}

// validScope 校验记忆作用域枚举。
func validScope(scope string) bool {
	switch scope {
	case model.ScopeUser, model.ScopeGroup, model.ScopeConversation, model.ScopeSession:
		return true
	default:
		return false
	}
}

// validType 校验记忆类型枚举。
func validType(memoryType string) bool {
	switch memoryType {
	case model.TypePreference, model.TypeSpeakingStyle, model.TypeLongTermGoal, model.TypeGroupProfile, model.TypeProjectState, model.TypeChatSummary, model.TypeAgentRun:
		return true
	default:
		return false
	}
}

// validVisibility 校验记忆可见性枚举。
func validVisibility(visibility string) bool {
	return visibility == model.VisibilityPrivate || visibility == model.VisibilityShared
}

// validVectorStatus 校验向量化状态枚举。
func validVectorStatus(status string) bool {
	return status == model.VectorPending || status == model.VectorDisabled || status == model.VectorReady
}

// defaultString 在 value 为空时返回 fallback。
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// minInt 返回两个整数中的较小值。
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// memoryFactsToDTO 批量将数据库模型转换为客户端 DTO。
func memoryFactsToDTO(facts []model.MemoryFact) []memoryclient.MemoryFact {
	out := make([]memoryclient.MemoryFact, 0, len(facts))
	for i := range facts {
		out = append(out, *memoryFactToDTO(&facts[i]))
	}
	return out
}

// memoryFactToDTO 将单条数据库记忆模型转换为客户端 DTO。
func memoryFactToDTO(fact *model.MemoryFact) *memoryclient.MemoryFact {
	if fact == nil {
		return nil
	}
	return &memoryclient.MemoryFact{
		ID:             fact.ID,
		BotID:          fact.BotID,
		UserID:         fact.UserID,
		OwnerUserID:    fact.OwnerUserID,
		GroupID:        fact.GroupID,
		ConversationID: fact.ConversationID,
		SessionID:      fact.SessionID,
		Scope:          fact.Scope,
		Type:           fact.Type,
		Title:          fact.Title,
		Content:        fact.Content,
		Source:         fact.Source,
		Visibility:     fact.Visibility,
		Enabled:        fact.Enabled,
		VectorStatus:   fact.VectorStatus,
		EmbeddingRef:   fact.EmbeddingRef,
		Confidence:     fact.Confidence,
		CreatedAt:      fact.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      fact.UpdatedAt.Format(time.RFC3339),
	}
}
