package service

import (
	"ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/model"
	"context"
	"strings"
	"testing"
	"time"
)

type fakeMemoryRepo struct {
	nextID int64
	facts  map[int64]*model.MemoryFact
}

func newFakeMemoryRepo() *fakeMemoryRepo {
	return &fakeMemoryRepo{nextID: 1, facts: map[int64]*model.MemoryFact{}}
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
	if fact.VectorStatus != model.VectorPending {
		t.Fatalf("vector status = %q, want pending", fact.VectorStatus)
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
