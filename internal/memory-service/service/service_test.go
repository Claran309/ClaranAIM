package service

import (
	"ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/model"
	"ClaranAIM/pkg/memoryclient"
	"context"
	"strings"
	"testing"
	"time"
)

type fakeMemoryRepo struct {
	nextID     int64
	facts      map[int64]*model.MemoryFact
	candidates map[int64]*model.MemoryCandidate
}

func newFakeMemoryRepo() *fakeMemoryRepo {
	return &fakeMemoryRepo{nextID: 1, facts: map[int64]*model.MemoryFact{}, candidates: map[int64]*model.MemoryCandidate{}}
}

func (r *fakeMemoryRepo) Create(ctx context.Context, fact *model.MemoryFact) error {
	if fact.ID == 0 {
		fact.ID = r.nextID
		r.nextID++
	}
	now := time.Now()
	fact.CreatedAt = now
	fact.UpdatedAt = now
	cp := *fact
	r.facts[fact.ID] = &cp
	return nil
}

func (r *fakeMemoryRepo) Update(ctx context.Context, fact *model.MemoryFact) error {
	cp := *fact
	cp.UpdatedAt = time.Now()
	r.facts[fact.ID] = &cp
	return nil
}

func (r *fakeMemoryRepo) GetByID(ctx context.Context, id int64) (*model.MemoryFact, error) {
	fact := r.facts[id]
	if fact == nil {
		return nil, nil
	}
	cp := *fact
	return &cp, nil
}

func (r *fakeMemoryRepo) List(ctx context.Context, filter dao.MemoryFilter) ([]model.MemoryFact, int64, error) {
	var out []model.MemoryFact
	for _, fact := range r.facts {
		if filter.BotID > 0 && fact.BotID != filter.BotID {
			continue
		}
		if filter.UserID > 0 && fact.UserID != filter.UserID {
			continue
		}
		if filter.OwnerUserID > 0 && fact.OwnerUserID != filter.OwnerUserID {
			continue
		}
		if filter.GroupID > 0 && fact.GroupID != filter.GroupID {
			continue
		}
		if filter.ConversationID > 0 && fact.ConversationID != filter.ConversationID {
			continue
		}
		if filter.SessionID != "" && fact.SessionID != filter.SessionID {
			continue
		}
		if len(filter.Scopes) > 0 && !containsString(filter.Scopes, fact.Scope) {
			continue
		}
		if !filter.IncludeDisabled && !fact.Enabled {
			continue
		}
		out = append(out, *fact)
	}
	return out, int64(len(out)), nil
}

func (r *fakeMemoryRepo) Delete(ctx context.Context, id int64) error {
	delete(r.facts, id)
	return nil
}

func (r *fakeMemoryRepo) Touch(ctx context.Context, ids []int64, at time.Time) error {
	for _, id := range ids {
		if fact := r.facts[id]; fact != nil {
			fact.LastUsedAt = &at
		}
	}
	return nil
}

func (r *fakeMemoryRepo) ListVisibleForRecall(ctx context.Context, filter dao.MemoryFilter) ([]model.MemoryFact, error) {
	facts, _, err := r.List(ctx, filter)
	return facts, err
}

func (r *fakeMemoryRepo) GetByIDs(ctx context.Context, ids []int64) ([]model.MemoryFact, error) {
	idSet := map[int64]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	var out []model.MemoryFact
	for _, fact := range r.facts {
		if idSet[fact.ID] {
			out = append(out, *fact)
		}
	}
	return out, nil
}

func (r *fakeMemoryRepo) CreateCandidate(ctx context.Context, candidate *model.MemoryCandidate) error {
	if candidate.ID == 0 {
		candidate.ID = r.nextID
		r.nextID++
	}
	now := time.Now()
	candidate.CreatedAt = now
	candidate.UpdatedAt = now
	cp := *candidate
	r.candidates[candidate.ID] = &cp
	return nil
}

func (r *fakeMemoryRepo) ListCandidates(ctx context.Context, filter dao.MemoryCandidateFilter) ([]model.MemoryCandidate, int64, error) {
	var out []model.MemoryCandidate
	for _, candidate := range r.candidates {
		if filter.BotID > 0 && candidate.BotID != filter.BotID {
			continue
		}
		if filter.UserID > 0 && candidate.UserID != filter.UserID {
			continue
		}
		if filter.OwnerUserID > 0 && candidate.OwnerUserID != filter.OwnerUserID {
			continue
		}
		if filter.Status != "" && candidate.Status != filter.Status {
			continue
		}
		out = append(out, *candidate)
	}
	return out, int64(len(out)), nil
}

func (r *fakeMemoryRepo) GetCandidateByID(ctx context.Context, id int64) (*model.MemoryCandidate, error) {
	candidate := r.candidates[id]
	if candidate == nil {
		return nil, nil
	}
	cp := *candidate
	return &cp, nil
}

func (r *fakeMemoryRepo) UpdateCandidate(ctx context.Context, candidate *model.MemoryCandidate) error {
	cp := *candidate
	cp.UpdatedAt = time.Now()
	r.candidates[candidate.ID] = &cp
	return nil
}

func TestCreateMemoryDefaultsPrivateVisibilityAndVectorPending(t *testing.T) {
	svc := NewMemoryService(newFakeMemoryRepo())
	fact, err := svc.CreateMemory(context.Background(), CreateMemoryInput{
		BotID:   1,
		UserID:  1001,
		Scope:   model.ScopeUser,
		Type:    model.TypePreference,
		Content: "用户喜欢简短直接的回答",
	})
	if err != nil {
		t.Fatalf("CreateMemory returned error: %v", err)
	}
	if fact.OwnerUserID != 1001 || fact.Visibility != model.VisibilityPrivate {
		t.Fatalf("fact owner/visibility = %d/%q, want private owner user", fact.OwnerUserID, fact.Visibility)
	}
	if fact.VectorStatus != model.VectorReady {
		t.Fatalf("vector status = %q, want ready after local vector indexing", fact.VectorStatus)
	}
}

func TestRecallMemoryRespectsBotUserConversationAndSessionIsolation(t *testing.T) {
	repo := newFakeMemoryRepo()
	svc := NewMemoryService(repo)
	_, _ = svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1001, ConversationID: 10, SessionID: "s1", Scope: model.ScopeUser, Type: model.TypePreference, Content: "喜欢中文"})
	_, _ = svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 2, UserID: 1001, ConversationID: 10, SessionID: "s1", Scope: model.ScopeUser, Type: model.TypePreference, Content: "其他Bot记忆"})
	_, _ = svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1002, ConversationID: 10, SessionID: "s1", Scope: model.ScopeUser, Type: model.TypePreference, Content: "其他用户记忆"})
	_, _ = svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1001, ConversationID: 11, SessionID: "s2", Scope: model.ScopeConversation, Type: model.TypeChatSummary, Content: "其他会话记忆"})
	_, _ = svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1001, ConversationID: 10, SessionID: "s2", Scope: model.ScopeSession, Type: model.TypeAgentRun, Content: "其他Session记忆"})

	result, err := svc.Recall(context.Background(), RecallInput{
		BotID:          1,
		UserID:         1001,
		ConversationID: 10,
		SessionID:      "s1",
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	text := result.ContextText
	if !strings.Contains(text, "喜欢中文") {
		t.Fatalf("recall text = %q, want matching memory", text)
	}
	if strings.Contains(text, "其他Bot") || strings.Contains(text, "其他用户") || strings.Contains(text, "其他会话") || strings.Contains(text, "其他Session") {
		t.Fatalf("recall leaked isolated memory: %q", text)
	}
}

func TestRecallUsesVectorMetadataMySQLFactCheckAndScoreFusion(t *testing.T) {
	repo := newFakeMemoryRepo()
	vector := newFakeMemoryVectorIndex()
	svc := NewMemoryServiceWithRAG(repo, vector, nil, MemoryRAGOptions{
		UseVector:          true,
		VectorCandidateK:   10,
		MinScore:           0.05,
		VectorWeight:       0.45,
		ImportanceWeight:   0.25,
		RecencyWeight:      0.15,
		ScopeWeight:        0.15,
		Now:                func() time.Time { return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC) },
		EnableLLMFiltering: false,
	})
	old := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	userFact := &model.MemoryFact{ID: 10, BotID: 1, UserID: 1001, OwnerUserID: 1001, Scope: model.ScopeUser, Type: model.TypePreference, Content: "用户喜欢先看结论", Visibility: model.VisibilityPrivate, Enabled: true, VectorStatus: model.VectorReady, Importance: 0.60, UpdatedAt: old}
	conversationFact := &model.MemoryFact{ID: 20, BotID: 1, UserID: 1001, OwnerUserID: 1001, ConversationID: 33, Scope: model.ScopeConversation, Type: model.TypeProjectState, Content: "当前项目正在重构 memory-service", Visibility: model.VisibilityPrivate, Enabled: true, VectorStatus: model.VectorReady, Importance: 0.90, UpdatedAt: recent}
	otherUserFact := &model.MemoryFact{ID: 30, BotID: 1, UserID: 1002, OwnerUserID: 1002, Scope: model.ScopeUser, Type: model.TypePreference, Content: "其他用户隐私", Visibility: model.VisibilityPrivate, Enabled: true, VectorStatus: model.VectorReady, Importance: 1.0, UpdatedAt: recent}
	repo.facts[userFact.ID] = userFact
	repo.facts[conversationFact.ID] = conversationFact
	repo.facts[otherUserFact.ID] = otherUserFact
	vector.hits = []MemoryVectorHit{
		{MemoryID: 30, Score: 0.99},
		{MemoryID: 10, Score: 0.55},
		{MemoryID: 20, Score: 0.50},
		{MemoryID: 999, Score: 0.98},
	}

	result, err := svc.Recall(context.Background(), RecallInput{
		BotID:          1,
		UserID:         1001,
		ConversationID: 33,
		Query:          "memory-service 怎么做 RAG 召回？",
		Limit:          2,
		MinScore:       0.05,
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if len(result.Facts) != 2 {
		t.Fatalf("facts len = %d, want 2 after MySQL fact check: %#v", len(result.Facts), result.Facts)
	}
	if result.Facts[0].ID != 20 {
		t.Fatalf("top memory id = %d, want conversation fact boosted over raw vector score", result.Facts[0].ID)
	}
	if strings.Contains(result.ContextText, "其他用户隐私") || strings.Contains(result.ContextText, "999") {
		t.Fatalf("context leaked unverified or invisible memory: %q", result.ContextText)
	}
	if !strings.Contains(result.ContextText, "以下是可能相关的长期记忆") || !strings.Contains(result.ContextText, "不要强行使用") || !strings.Contains(result.ContextText, "用户当前输入优先级高于记忆") {
		t.Fatalf("context text missing prompt injection policy: %q", result.ContextText)
	}
	if result.Facts[0].FinalScore <= result.Facts[0].VectorScore {
		t.Fatalf("final score should include fusion beyond vector score: %#v", result.Facts[0])
	}
}

func TestRecallOptionalLLMFilterRemovesNoisyCandidates(t *testing.T) {
	repo := newFakeMemoryRepo()
	filter := &fakeMemoryRelevanceFilter{keepIDs: map[int64]bool{2: true}}
	svc := NewMemoryServiceWithRAG(repo, nil, filter, MemoryRAGOptions{EnableLLMFiltering: true, MinScore: 0, Now: func() time.Time { return time.Now() }})
	_, _ = svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1001, Scope: model.ScopeUser, Type: model.TypePreference, Content: "噪声记忆", Importance: 0.9})
	_, _ = svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1001, Scope: model.ScopeUser, Type: model.TypePreference, Content: "真正有用的记忆", Importance: 0.8})

	result, err := svc.Recall(context.Background(), RecallInput{BotID: 1, UserID: 1001, Query: "需要有用记忆", Limit: 10})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if len(result.Facts) != 1 || result.Facts[0].ID != 2 {
		t.Fatalf("filtered facts = %#v, want only fact 2", result.Facts)
	}
}

func TestCreateCandidateStoresPendingMemoryForGovernance(t *testing.T) {
	repo := newFakeMemoryRepo()
	svc := NewMemoryService(repo)

	candidate, err := svc.CreateCandidate(context.Background(), CandidateInput{
		BotID:          1,
		UserID:         1001,
		OwnerUserID:    1001,
		ConversationID: 33,
		Scope:          model.ScopeConversation,
		Type:           model.TypeProjectState,
		Title:          "项目状态",
		Content:        "用户正在重做 memory-service",
		Source:         "agent_extract",
		Confidence:     0.72,
		Evidence:       "聊天中反复提到 memory-service RAG 化",
	})
	if err != nil {
		t.Fatalf("CreateCandidate returned error: %v", err)
	}
	if candidate.Status != model.CandidatePending {
		t.Fatalf("candidate status = %q, want pending", candidate.Status)
	}
	if len(repo.candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(repo.candidates))
	}
}

func TestCreateCandidateAllowsSystemConversationMemoryWithoutAgent(t *testing.T) {
	repo := newFakeMemoryRepo()
	svc := NewMemoryService(repo)

	candidate, err := svc.CreateCandidate(context.Background(), CandidateInput{
		BotID:          0,
		UserID:         1001,
		OwnerUserID:    1001,
		ConversationID: 300,
		Scope:          model.ScopeConversation,
		Type:           model.TypeChatSummary,
		Title:          "会话候选记忆",
		Content:        "这段会话决定把聊天摘要写入长期 RAG。",
		Source:         "conversation-intelligence",
	})
	if err != nil {
		t.Fatalf("CreateCandidate returned error: %v", err)
	}
	if candidate.BotID != 0 || candidate.Status != model.CandidatePending {
		t.Fatalf("unexpected system candidate: %#v", candidate)
	}
}

func TestListCandidatesUsesViewerScopeAndAllowsAdminGlobalView(t *testing.T) {
	repo := newFakeMemoryRepo()
	svc := NewMemoryService(repo)
	ownerOne, _ := svc.CreateCandidate(context.Background(), CandidateInput{BotID: 1, UserID: 1001, OwnerUserID: 1001, Scope: model.ScopeUser, Type: model.TypePreference, Content: "用户一候选"})
	ownerTwo, _ := svc.CreateCandidate(context.Background(), CandidateInput{BotID: 1, UserID: 1002, OwnerUserID: 1002, Scope: model.ScopeUser, Type: model.TypePreference, Content: "用户二候选"})

	userCandidates, userTotal, err := svc.ListCandidates(context.Background(), 1001, memoryclient.CandidateFilter{})
	if err != nil {
		t.Fatalf("ListCandidates user scope returned error: %v", err)
	}
	if userTotal != 1 || len(userCandidates) != 1 || userCandidates[0].ID != ownerOne.ID {
		t.Fatalf("user candidates = %#v total=%d, want only owner one", userCandidates, userTotal)
	}

	adminCandidates, adminTotal, err := svc.ListCandidates(context.Background(), 0, memoryclient.CandidateFilter{})
	if err != nil {
		t.Fatalf("ListCandidates admin global returned error: %v", err)
	}
	if adminTotal != 2 || len(adminCandidates) != 2 {
		t.Fatalf("admin candidates = %#v total=%d, want all candidates including %d and %d", adminCandidates, adminTotal, ownerOne.ID, ownerTwo.ID)
	}
}

func TestAdminReviewCandidateCanHandleOtherOwnersWithoutUserScopeLeak(t *testing.T) {
	repo := newFakeMemoryRepo()
	svc := NewMemoryService(repo)
	candidate, _ := svc.CreateCandidate(context.Background(), CandidateInput{BotID: 1, UserID: 1002, OwnerUserID: 1002, Scope: model.ScopeUser, Type: model.TypePreference, Content: "用户二候选"})

	if _, err := svc.AcceptCandidate(context.Background(), 1001, candidate.ID); err == nil {
		t.Fatal("expected normal user to be unable to accept another owner's candidate")
	}
	accepted, err := svc.AcceptCandidate(context.Background(), -9999, candidate.ID)
	if err != nil {
		t.Fatalf("admin review AcceptCandidate returned error: %v", err)
	}
	if accepted.Status != model.CandidateAccepted || accepted.AcceptedMemoryID == 0 {
		t.Fatalf("accepted candidate = %#v, want accepted by admin review path", accepted)
	}
}

func TestAcceptCandidateExpiresConflictingOldMemoryAndKeepsTimeline(t *testing.T) {
	repo := newFakeMemoryRepo()
	svc := NewMemoryService(repo)
	old, _ := svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1001, Scope: model.ScopeUser, Type: model.TypePreference, Title: "Kafka 学习状态", Content: "用户不懂 Kafka", Importance: 0.8})
	candidate, _ := svc.CreateCandidate(context.Background(), CandidateInput{
		BotID:              1,
		UserID:             1001,
		OwnerUserID:        1001,
		Scope:              model.ScopeUser,
		Type:               model.TypePreference,
		Title:              "Kafka 学习状态",
		Content:            "用户已经学完 Kafka 基础",
		ConflictMemoryIDs:  []int64{old.ID},
		ConflictResolution: "supersede",
	})

	accepted, err := svc.AcceptCandidate(context.Background(), 1001, candidate.ID)
	if err != nil {
		t.Fatalf("AcceptCandidate returned error: %v", err)
	}
	if accepted.Status != model.CandidateAccepted || accepted.AcceptedMemoryID == 0 {
		t.Fatalf("accepted candidate = %#v, want accepted with linked memory", accepted)
	}
	oldFact := repo.facts[old.ID]
	if oldFact.Enabled || oldFact.ExpiredAt == nil || oldFact.SupersededBy != accepted.AcceptedMemoryID {
		t.Fatalf("old conflicting memory = %#v, want expired and superseded", oldFact)
	}
	newFact := repo.facts[accepted.AcceptedMemoryID]
	if newFact.PreviousMemoryID != old.ID || newFact.Content != "用户已经学完 Kafka 基础" {
		t.Fatalf("new memory = %#v, want timeline link to old memory", newFact)
	}
}

func TestUserCanUpdateDeleteAndDisableOwnMemoryOnly(t *testing.T) {
	repo := newFakeMemoryRepo()
	svc := NewMemoryService(repo)
	owned, _ := svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1001, Scope: model.ScopeUser, Type: model.TypePreference, Content: "旧记忆"})
	other, _ := svc.CreateMemory(context.Background(), CreateMemoryInput{BotID: 1, UserID: 1002, Scope: model.ScopeUser, Type: model.TypePreference, Content: "别人的记忆"})

	updated, err := svc.UpdateMemory(context.Background(), 1001, owned.ID, UpdateMemoryInput{Content: "新记忆", Enabled: boolPtr(false), Type: model.TypeLongTermGoal})
	if err != nil {
		t.Fatalf("UpdateMemory returned error: %v", err)
	}
	if updated.Content != "新记忆" || updated.Enabled {
		t.Fatalf("updated fact = %#v, want disabled new content", updated)
	}
	if _, err := svc.UpdateMemory(context.Background(), 1001, other.ID, UpdateMemoryInput{Content: "偷改"}); err == nil {
		t.Fatal("expected updating another user's memory to fail")
	}
	if err := svc.DeleteMemory(context.Background(), 1001, other.ID); err == nil {
		t.Fatal("expected deleting another user's memory to fail")
	}
	if err := svc.DeleteMemory(context.Background(), 1001, owned.ID); err != nil {
		t.Fatalf("DeleteMemory returned error: %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func boolPtr(v bool) *bool { return &v }

type fakeMemoryVectorIndex struct {
	hits    []MemoryVectorHit
	upserts []int64
}

func newFakeMemoryVectorIndex() *fakeMemoryVectorIndex {
	return &fakeMemoryVectorIndex{}
}

func (v *fakeMemoryVectorIndex) Upsert(ctx context.Context, memoryID int64, text string, metadata MemoryVectorMetadata) error {
	v.upserts = append(v.upserts, memoryID)
	return nil
}

func (v *fakeMemoryVectorIndex) Search(ctx context.Context, query string, filter MemoryVectorFilter, limit int) ([]MemoryVectorHit, error) {
	return v.hits, nil
}

type fakeMemoryRelevanceFilter struct {
	keepIDs map[int64]bool
}

func (f *fakeMemoryRelevanceFilter) Filter(ctx context.Context, query string, candidates []ScoredMemory) ([]ScoredMemory, error) {
	var out []ScoredMemory
	for _, candidate := range candidates {
		if f.keepIDs[candidate.Fact.ID] {
			out = append(out, candidate)
		}
	}
	return out, nil
}
