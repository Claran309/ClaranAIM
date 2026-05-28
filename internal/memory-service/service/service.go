// Package service implements editable Agent memory facts and recall rules.
package service

import (
	"ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/model"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MemoryService exposes the Phase4 memory operations used by API gateway and Agent runtime callers.
type MemoryService interface {
	CreateMemory(ctx context.Context, input CreateMemoryInput) (*model.MemoryFact, error)
	ListMemories(ctx context.Context, viewerID int64, filter dao.MemoryFilter) ([]model.MemoryFact, int64, error)
	Recall(ctx context.Context, input RecallInput) (RecallResult, error)
	UpdateMemory(ctx context.Context, viewerID, memoryID int64, input UpdateMemoryInput) (*model.MemoryFact, error)
	DeleteMemory(ctx context.Context, viewerID, memoryID int64) error
}

// CreateMemoryInput is the user/API-facing shape for one memory fact.
type CreateMemoryInput struct {
	BotID          int64
	UserID         int64
	OwnerUserID    int64
	GroupID        int64
	ConversationID int64
	SessionID      string
	Scope          string
	Type           string
	Title          string
	Content        string
	Source         string
	Visibility     string
	Enabled        *bool
	VectorStatus   string
	EmbeddingRef   string
	Confidence     float64
}

// UpdateMemoryInput contains optional mutable fields for user-governed memory.
type UpdateMemoryInput struct {
	Scope        string
	Type         string
	Title        string
	Content      string
	Source       string
	Visibility   string
	Enabled      *bool
	VectorStatus string
	EmbeddingRef string
	Confidence   *float64
}

// RecallInput scopes memory retrieval before injecting facts into an Agent context.
type RecallInput struct {
	BotID          int64
	UserID         int64
	GroupID        int64
	ConversationID int64
	SessionID      string
	Limit          int
}

// RecallResult keeps recalled facts plus a ready-to-inject text block.
type RecallResult struct {
	Facts       []model.MemoryFact
	ContextText string
}

type memoryServiceImpl struct {
	repo dao.MemoryRepository
}

// NewMemoryService creates the memory business service.
func NewMemoryService(repo dao.MemoryRepository) MemoryService {
	return &memoryServiceImpl{repo: repo}
}

// CreateMemory validates and stores a memory fact. User profile data defaults
// to private visibility because speaking habits and preferences are personal.
func (s *memoryServiceImpl) CreateMemory(ctx context.Context, input CreateMemoryInput) (*model.MemoryFact, error) {
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
	return fact, nil
}

// ListMemories returns only memory owned by the current user unless a caller has
// already scoped the filter tighter. This protects private profile memories from
// accidental cross-user listing.
func (s *memoryServiceImpl) ListMemories(ctx context.Context, viewerID int64, filter dao.MemoryFilter) ([]model.MemoryFact, int64, error) {
	if s.repo == nil {
		return nil, 0, errors.New("memory repository未配置")
	}
	if viewerID <= 0 {
		return nil, 0, errors.New("用户未登录")
	}
	filter.OwnerUserID = viewerID
	return s.repo.List(ctx, filter)
}

// Recall retrieves only facts that match the current Agent, user and context
// boundary. Broad user facts may cross conversations, but conversation/session
// facts must match exactly to avoid memory串线.
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
	return RecallResult{Facts: facts, ContextText: FormatMemoryContext(facts)}, nil
}

// UpdateMemory modifies an owned memory fact. Private governance is explicit:
// users can update or close their own memory but cannot edit others' facts.
func (s *memoryServiceImpl) UpdateMemory(ctx context.Context, viewerID, memoryID int64, input UpdateMemoryInput) (*model.MemoryFact, error) {
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
	return fact, nil
}

// DeleteMemory removes an owned memory fact.
func (s *memoryServiceImpl) DeleteMemory(ctx context.Context, viewerID, memoryID int64) error {
	_, err := s.loadOwnedMemory(ctx, viewerID, memoryID)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, memoryID)
}

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

// FormatMemoryContext converts recalled facts into a compact prompt section.
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

func validScope(scope string) bool {
	switch scope {
	case model.ScopeUser, model.ScopeGroup, model.ScopeConversation, model.ScopeSession:
		return true
	default:
		return false
	}
}

func validType(memoryType string) bool {
	switch memoryType {
	case model.TypePreference, model.TypeSpeakingStyle, model.TypeLongTermGoal, model.TypeGroupProfile, model.TypeProjectState, model.TypeChatSummary, model.TypeAgentRun:
		return true
	default:
		return false
	}
}

func validVisibility(visibility string) bool {
	return visibility == model.VisibilityPrivate || visibility == model.VisibilityShared
}

func validVectorStatus(status string) bool {
	return status == model.VectorPending || status == model.VectorDisabled || status == model.VectorReady
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
