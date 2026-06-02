// Package service 实现可由用户治理的 Agent 记忆事实和召回规则。
package service

import (
	"ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/model"
	"ClaranAIM/pkg/memoryclient"
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
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
	CreateCandidate(ctx context.Context, input CandidateInput) (*memoryclient.MemoryCandidate, error)
	ListCandidates(ctx context.Context, viewerID int64, filter memoryclient.CandidateFilter) ([]memoryclient.MemoryCandidate, int64, error)
	AcceptCandidate(ctx context.Context, viewerID, candidateID int64) (*memoryclient.MemoryCandidate, error)
	RejectCandidate(ctx context.Context, viewerID, candidateID int64) (*memoryclient.MemoryCandidate, error)
}

// CreateMemoryInput 是创建单条记忆事实的 API 入参结构。
type CreateMemoryInput = memoryclient.CreateMemoryInput

// UpdateMemoryInput 保存用户可编辑记忆的可选更新字段。
type UpdateMemoryInput = memoryclient.UpdateMemoryInput

// RecallInput 限定记忆召回边界，避免把不相关用户、群或会话的记忆注入 Agent。
type RecallInput = memoryclient.RecallInput

// RecallResult 保存召回到的记忆事实，以及可直接拼入 prompt 的文本块。
type RecallResult = memoryclient.RecallResult
type CandidateInput = memoryclient.CandidateInput

// memoryServiceImpl 是 MemoryService 的默认实现，负责参数校验、权限裁剪和 DTO 转换。
type memoryServiceImpl struct {
	repo      dao.MemoryRepository
	vector    MemoryVectorIndex
	filter    MemoryRelevanceFilter
	ragOption MemoryRAGOptions
}

// NewMemoryService 创建记忆业务服务。
func NewMemoryService(repo dao.MemoryRepository) MemoryService {
	return NewMemoryServiceWithRAG(repo, nil, nil, MemoryRAGOptions{})
}

// NewMemoryServiceWithRAG 创建带向量召回、融合打分和可选 LLM 过滤的记忆服务。
func NewMemoryServiceWithRAG(repo dao.MemoryRepository, vector MemoryVectorIndex, filter MemoryRelevanceFilter, opts MemoryRAGOptions) MemoryService {
	opts = normalizeMemoryRAGOptions(opts)
	if vector == nil {
		vector = NewLocalMemoryVectorIndex(HashMemoryEmbeddingProvider{Dim: 256})
	}
	return &memoryServiceImpl{repo: repo, vector: vector, filter: filter, ragOption: opts}
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
		BotID:            input.BotID,
		UserID:           input.UserID,
		OwnerUserID:      ownerID,
		GroupID:          input.GroupID,
		ConversationID:   input.ConversationID,
		SessionID:        strings.TrimSpace(input.SessionID),
		Scope:            input.Scope,
		Type:             input.Type,
		Title:            strings.TrimSpace(input.Title),
		Content:          strings.TrimSpace(input.Content),
		Source:           strings.TrimSpace(input.Source),
		Visibility:       visibility,
		Enabled:          enabled,
		VectorStatus:     vectorStatus,
		EmbeddingRef:     strings.TrimSpace(input.EmbeddingRef),
		Confidence:       input.Confidence,
		Importance:       normalizeScore(input.Importance, 0.5),
		PreviousMemoryID: input.PreviousMemoryID,
	}
	if err := s.repo.Create(ctx, fact); err != nil {
		return nil, err
	}
	s.indexMemory(ctx, fact)
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
	limit := normalizeRecallLimit(input.Limit)
	minScore := input.MinScore
	if minScore <= 0 {
		minScore = s.ragOption.MinScore
	}
	candidates, err := s.collectRecallCandidates(ctx, input)
	if err != nil {
		return RecallResult{}, err
	}
	scored := scoreRecallCandidates(candidates, input, s.ragOption)
	scored = filterMinScore(scored, minScore)
	if input.UseLLMFilter || s.ragOption.EnableLLMFiltering {
		scored = s.applyRelevanceFilter(ctx, input.Query, scored)
	}
	if len(scored) > limit {
		scored = scored[:limit]
	}
	facts := make([]model.MemoryFact, 0, len(scored))
	for _, item := range scored {
		fact := item.Fact
		fact.VectorScore = item.VectorScore
		fact.FinalScore = item.FinalScore
		fact.ScoreReason = item.Reason
		facts = append(facts, fact)
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
	if input.Importance != nil {
		fact.Importance = normalizeScore(*input.Importance, fact.Importance)
	}
	if err := validateFact(*fact); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, fact); err != nil {
		return nil, err
	}
	s.indexMemory(ctx, fact)
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

// CreateCandidate 保存一条 pending 候选记忆，等待用户确认或规则接受。
func (s *memoryServiceImpl) CreateCandidate(ctx context.Context, input CandidateInput) (*memoryclient.MemoryCandidate, error) {
	if s.repo == nil {
		return nil, errors.New("memory repository未配置")
	}
	if input.BotID < 0 || input.UserID <= 0 {
		return nil, errors.New("bot_id不能为负且user_id不能为空")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, errors.New("候选记忆内容不能为空")
	}
	if !validScope(input.Scope) || !validType(input.Type) {
		return nil, errors.New("候选记忆范围或类型无效")
	}
	ownerID := input.OwnerUserID
	if ownerID == 0 {
		ownerID = input.UserID
	}
	resolution := strings.TrimSpace(input.ConflictResolution)
	if resolution == "" {
		resolution = model.ConflictKeep
	}
	candidate := &model.MemoryCandidate{
		BotID:              input.BotID,
		UserID:             input.UserID,
		OwnerUserID:        ownerID,
		GroupID:            input.GroupID,
		ConversationID:     input.ConversationID,
		SessionID:          strings.TrimSpace(input.SessionID),
		Scope:              input.Scope,
		Type:               input.Type,
		Title:              strings.TrimSpace(input.Title),
		Content:            strings.TrimSpace(input.Content),
		Source:             strings.TrimSpace(input.Source),
		Evidence:           strings.TrimSpace(input.Evidence),
		Confidence:         input.Confidence,
		Importance:         normalizeScore(input.Importance, 0.5),
		Status:             model.CandidatePending,
		ConflictMemoryIDs:  model.EncodeIDList(input.ConflictMemoryIDs),
		ConflictResolution: resolution,
	}
	if err := s.repo.CreateCandidate(ctx, candidate); err != nil {
		return nil, err
	}
	return candidateToDTO(candidate), nil
}

// ListCandidates 返回候选记忆列表，用于用户治理页或 admin-service 审核台。
// viewerID > 0 时强制按 owner_user_id 裁剪；viewerID == 0 是内部管理入口的全局查询约定，
// api-gateway 的普通用户入口会注入真实用户 ID，不能直接拿到全局候选。
func (s *memoryServiceImpl) ListCandidates(ctx context.Context, viewerID int64, filter memoryclient.CandidateFilter) ([]memoryclient.MemoryCandidate, int64, error) {
	if s.repo == nil {
		return nil, 0, errors.New("memory repository未配置")
	}
	ownerID := viewerID
	if viewerID < 0 {
		return nil, 0, errors.New("用户未登录")
	}
	candidates, total, err := s.repo.ListCandidates(ctx, dao.MemoryCandidateFilter{
		BotID:       filter.BotID,
		UserID:      filter.UserID,
		OwnerUserID: ownerID,
		Status:      filter.Status,
		Limit:       filter.Limit,
		Offset:      filter.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	return candidatesToDTO(candidates), total, nil
}

// AcceptCandidate 把 pending 候选转为正式记忆，并按冲突策略处理旧记忆。
func (s *memoryServiceImpl) AcceptCandidate(ctx context.Context, viewerID, candidateID int64) (*memoryclient.MemoryCandidate, error) {
	candidate, err := s.loadOwnedCandidate(ctx, viewerID, candidateID)
	if err != nil {
		return nil, err
	}
	if candidate.Status != model.CandidatePending {
		return nil, errors.New("只能接受pending候选记忆")
	}
	conflicts := model.DecodeIDList(candidate.ConflictMemoryIDs)
	previousID := int64(0)
	if len(conflicts) > 0 {
		previousID = conflicts[0]
	}
	fact, err := s.CreateMemory(ctx, CreateMemoryInput{
		BotID:            candidate.BotID,
		UserID:           candidate.UserID,
		OwnerUserID:      candidate.OwnerUserID,
		GroupID:          candidate.GroupID,
		ConversationID:   candidate.ConversationID,
		SessionID:        candidate.SessionID,
		Scope:            candidate.Scope,
		Type:             candidate.Type,
		Title:            candidate.Title,
		Content:          candidate.Content,
		Source:           candidate.Source,
		Visibility:       model.VisibilityPrivate,
		VectorStatus:     model.VectorPending,
		Confidence:       candidate.Confidence,
		Importance:       candidate.Importance,
		PreviousMemoryID: previousID,
	})
	if err != nil {
		return nil, err
	}
	if candidate.ConflictResolution == model.ConflictSupersede || candidate.ConflictResolution == model.ConflictWeaken {
		s.resolveConflicts(ctx, conflicts, fact.ID, candidate.ConflictResolution)
	}
	candidate.Status = model.CandidateAccepted
	candidate.AcceptedMemoryID = fact.ID
	if err := s.repo.UpdateCandidate(ctx, candidate); err != nil {
		return nil, err
	}
	return candidateToDTO(candidate), nil
}

// RejectCandidate 拒绝候选记忆，不写入 memory_facts。
func (s *memoryServiceImpl) RejectCandidate(ctx context.Context, viewerID, candidateID int64) (*memoryclient.MemoryCandidate, error) {
	candidate, err := s.loadOwnedCandidate(ctx, viewerID, candidateID)
	if err != nil {
		return nil, err
	}
	if candidate.Status != model.CandidatePending {
		return nil, errors.New("只能拒绝pending候选记忆")
	}
	candidate.Status = model.CandidateRejected
	if err := s.repo.UpdateCandidate(ctx, candidate); err != nil {
		return nil, err
	}
	return candidateToDTO(candidate), nil
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

func (s *memoryServiceImpl) loadOwnedCandidate(ctx context.Context, viewerID, candidateID int64) (*model.MemoryCandidate, error) {
	if s.repo == nil {
		return nil, errors.New("memory repository未配置")
	}
	adminReview := viewerID < 0
	if viewerID == 0 {
		return nil, errors.New("用户未登录")
	}
	candidate, err := s.repo.GetCandidateByID(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	if candidate == nil {
		return nil, errors.New("候选记忆不存在")
	}
	if !adminReview && candidate.OwnerUserID != viewerID {
		return nil, errors.New("只能管理自己的候选记忆")
	}
	return candidate, nil
}

// validateCreateInput 校验创建记忆所需的必填字段和枚举值。
func validateCreateInput(input CreateMemoryInput) error {
	if input.BotID < 0 {
		return errors.New("bot_id不能为负")
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
	b.WriteString("以下是可能相关的长期记忆。\n")
	b.WriteString("如果和当前问题无关，不要强行使用。用户当前输入优先级高于记忆。\n")
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
	case model.TypePreference, model.TypeSpeakingStyle, model.TypeLongTermGoal, model.TypeGroupProfile, model.TypeProjectState, model.TypeChatSummary, model.TypeAgentRun, model.TypeLearningState, model.TypeRepeatedIssue:
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

func normalizeMemoryRAGOptions(opts MemoryRAGOptions) MemoryRAGOptions {
	if opts.VectorCandidateK <= 0 {
		opts.VectorCandidateK = 80
	}
	if opts.MinScore <= 0 {
		opts.MinScore = 0.05
	}
	if opts.VectorWeight <= 0 && opts.ImportanceWeight <= 0 && opts.RecencyWeight <= 0 && opts.ScopeWeight <= 0 {
		opts.VectorWeight = 0.45
		opts.ImportanceWeight = 0.25
		opts.RecencyWeight = 0.15
		opts.ScopeWeight = 0.15
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func normalizeRecallLimit(limit int) int {
	if limit <= 0 {
		return 12
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func (s *memoryServiceImpl) collectRecallCandidates(ctx context.Context, input RecallInput) ([]ScoredMemory, error) {
	vectorScores := map[int64]float64{}
	var vectorIDs []int64
	query := strings.TrimSpace(input.Query)
	vectorK := input.VectorCandidateK
	if vectorK <= 0 {
		vectorK = s.ragOption.VectorCandidateK
	}
	if s.vector != nil && s.ragOption.UseVector && query != "" {
		hits, err := s.vector.Search(ctx, query, MemoryVectorFilter{BotID: input.BotID, UserID: input.UserID, GroupID: input.GroupID, ConversationID: input.ConversationID, SessionID: input.SessionID}, vectorK)
		if err == nil {
			for _, hit := range hits {
				if hit.MemoryID <= 0 {
					continue
				}
				vectorScores[hit.MemoryID] = hit.Score
				vectorIDs = append(vectorIDs, hit.MemoryID)
			}
		}
	}
	factByID := map[int64]model.MemoryFact{}
	if len(vectorIDs) > 0 {
		facts, err := s.repo.GetByIDs(ctx, vectorIDs)
		if err != nil {
			return nil, err
		}
		for _, fact := range facts {
			if recallMatchesContext(fact, input) && factVisibleForRecall(fact, input) {
				factByID[fact.ID] = fact
			}
		}
	}
	facts, err := s.repo.ListVisibleForRecall(ctx, dao.MemoryFilter{
		BotID:           input.BotID,
		UserID:          input.UserID,
		IncludeDisabled: false,
		Limit:           120,
	})
	if err != nil {
		return nil, err
	}
	for _, fact := range facts {
		if recallMatchesContext(fact, input) && factVisibleForRecall(fact, input) {
			factByID[fact.ID] = fact
		}
	}
	out := make([]ScoredMemory, 0, len(factByID))
	for _, fact := range factByID {
		out = append(out, ScoredMemory{Fact: fact, VectorScore: vectorScores[fact.ID]})
	}
	return out, nil
}

func factVisibleForRecall(fact model.MemoryFact, input RecallInput) bool {
	if !fact.Enabled || fact.ExpiredAt != nil {
		return false
	}
	return fact.BotID == input.BotID && fact.UserID == input.UserID
}

func scoreRecallCandidates(candidates []ScoredMemory, input RecallInput, opts MemoryRAGOptions) []ScoredMemory {
	now := opts.Now()
	for i := range candidates {
		fact := candidates[i].Fact
		vectorScore := candidates[i].VectorScore
		if vectorScore == 0 && strings.TrimSpace(input.Query) != "" {
			vectorScore = lexicalMemoryScore(input.Query, fact.Content+" "+fact.Title)
		}
		importance := normalizeScore(fact.Importance, 0.5)
		recency := recencyScore(fact.UpdatedAt, now)
		scope := scopeBoost(fact, input)
		final := opts.VectorWeight*vectorScore + opts.ImportanceWeight*importance + opts.RecencyWeight*recency + opts.ScopeWeight*scope
		candidates[i].VectorScore = roundScore(vectorScore)
		candidates[i].FinalScore = roundScore(final)
		candidates[i].Reason = fmt.Sprintf("vector=%.3f importance=%.3f recency=%.3f scope=%.3f", vectorScore, importance, recency, scope)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].FinalScore == candidates[j].FinalScore {
			return candidates[i].Fact.UpdatedAt.After(candidates[j].Fact.UpdatedAt)
		}
		return candidates[i].FinalScore > candidates[j].FinalScore
	})
	return candidates
}

func filterMinScore(candidates []ScoredMemory, minScore float64) []ScoredMemory {
	if minScore <= 0 {
		return candidates
	}
	out := make([]ScoredMemory, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.FinalScore >= minScore {
			out = append(out, candidate)
		}
	}
	return out
}

func (s *memoryServiceImpl) applyRelevanceFilter(ctx context.Context, query string, candidates []ScoredMemory) []ScoredMemory {
	if s.filter == nil || strings.TrimSpace(query) == "" || len(candidates) == 0 {
		return candidates
	}
	filtered, err := s.filter.Filter(ctx, query, candidates)
	if err != nil {
		return candidates
	}
	return filtered
}

func (s *memoryServiceImpl) indexMemory(ctx context.Context, fact *model.MemoryFact) {
	if s.vector == nil || fact == nil || !fact.Enabled || fact.VectorStatus == model.VectorDisabled {
		return
	}
	_ = s.vector.Upsert(ctx, fact.ID, strings.TrimSpace(fact.Title+"\n"+fact.Content), MemoryVectorMetadata{
		BotID:          fact.BotID,
		UserID:         fact.UserID,
		OwnerUserID:    fact.OwnerUserID,
		GroupID:        fact.GroupID,
		ConversationID: fact.ConversationID,
		SessionID:      fact.SessionID,
		Scope:          fact.Scope,
		Type:           fact.Type,
		Visibility:     fact.Visibility,
	})
	fact.VectorStatus = model.VectorReady
	fact.EmbeddingRef = fmt.Sprintf("memory:%d", fact.ID)
	_ = s.repo.Update(ctx, fact)
}

func (s *memoryServiceImpl) resolveConflicts(ctx context.Context, ids []int64, newID int64, strategy string) {
	now := time.Now()
	for _, id := range ids {
		fact, err := s.repo.GetByID(ctx, id)
		if err != nil || fact == nil {
			continue
		}
		switch strategy {
		case model.ConflictSupersede:
			fact.Enabled = false
			fact.ExpiredAt = &now
			fact.SupersededBy = newID
		case model.ConflictWeaken:
			fact.Importance = fact.Importance * 0.5
			fact.SupersededBy = newID
		}
		_ = s.repo.Update(ctx, fact)
	}
}

func lexicalMemoryScore(query, content string) float64 {
	q := strings.Fields(strings.ToLower(query))
	c := strings.ToLower(content)
	if len(q) == 0 || c == "" {
		return 0
	}
	hits := 0
	for _, token := range q {
		if strings.Contains(c, token) {
			hits++
		}
	}
	return float64(hits) / float64(len(q))
}

func recencyScore(updatedAt, now time.Time) float64 {
	if updatedAt.IsZero() {
		return 0.2
	}
	days := now.Sub(updatedAt).Hours() / 24
	if days <= 1 {
		return 1
	}
	score := 1 / (1 + days/30)
	return normalizeScore(score, 0.2)
}

func scopeBoost(fact model.MemoryFact, input RecallInput) float64 {
	switch fact.Scope {
	case model.ScopeSession:
		if input.SessionID != "" && fact.SessionID == input.SessionID {
			return 1
		}
	case model.ScopeConversation:
		if input.ConversationID > 0 && fact.ConversationID == input.ConversationID {
			return 0.9
		}
	case model.ScopeGroup:
		if input.GroupID > 0 && fact.GroupID == input.GroupID {
			return 0.75
		}
	case model.ScopeUser:
		return 0.55
	}
	return 0
}

func normalizeScore(value, fallback float64) float64 {
	if value <= 0 {
		value = fallback
	}
	if value > 1 {
		return 1
	}
	return value
}

func roundScore(value float64) float64 {
	return math.Round(value*10000) / 10000
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
		ID:               fact.ID,
		BotID:            fact.BotID,
		UserID:           fact.UserID,
		OwnerUserID:      fact.OwnerUserID,
		GroupID:          fact.GroupID,
		ConversationID:   fact.ConversationID,
		SessionID:        fact.SessionID,
		Scope:            fact.Scope,
		Type:             fact.Type,
		Title:            fact.Title,
		Content:          fact.Content,
		Source:           fact.Source,
		Visibility:       fact.Visibility,
		Enabled:          fact.Enabled,
		VectorStatus:     fact.VectorStatus,
		EmbeddingRef:     fact.EmbeddingRef,
		Confidence:       fact.Confidence,
		Importance:       fact.Importance,
		VectorScore:      fact.VectorScore,
		FinalScore:       fact.FinalScore,
		ScoreReason:      fact.ScoreReason,
		ExpiredAt:        formatOptionalTime(fact.ExpiredAt),
		SupersededBy:     fact.SupersededBy,
		PreviousMemoryID: fact.PreviousMemoryID,
		CreatedAt:        fact.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        fact.UpdatedAt.Format(time.RFC3339),
	}
}

func candidatesToDTO(candidates []model.MemoryCandidate) []memoryclient.MemoryCandidate {
	out := make([]memoryclient.MemoryCandidate, 0, len(candidates))
	for i := range candidates {
		out = append(out, *candidateToDTO(&candidates[i]))
	}
	return out
}

func candidateToDTO(candidate *model.MemoryCandidate) *memoryclient.MemoryCandidate {
	if candidate == nil {
		return nil
	}
	return &memoryclient.MemoryCandidate{
		ID:                 candidate.ID,
		BotID:              candidate.BotID,
		UserID:             candidate.UserID,
		OwnerUserID:        candidate.OwnerUserID,
		GroupID:            candidate.GroupID,
		ConversationID:     candidate.ConversationID,
		SessionID:          candidate.SessionID,
		Scope:              candidate.Scope,
		Type:               candidate.Type,
		Title:              candidate.Title,
		Content:            candidate.Content,
		Source:             candidate.Source,
		Evidence:           candidate.Evidence,
		Confidence:         candidate.Confidence,
		Importance:         candidate.Importance,
		Status:             candidate.Status,
		ConflictMemoryIDs:  model.DecodeIDList(candidate.ConflictMemoryIDs),
		ConflictResolution: candidate.ConflictResolution,
		AcceptedMemoryID:   candidate.AcceptedMemoryID,
		CreatedAt:          candidate.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          candidate.UpdatedAt.Format(time.RFC3339),
	}
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
