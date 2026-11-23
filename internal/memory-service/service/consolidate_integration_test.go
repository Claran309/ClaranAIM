//go:build integration

package service

import (
	"ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/model"
	"context"
	"os"
	"testing"
	"time"
)

const memoryIntegrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupMemoryIntegration(t *testing.T) (MemoryService, dao.MemoryRepository) {
	t.Helper()
	dsn := os.Getenv(memoryIntegrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 memory MySQL 集成测试", memoryIntegrationDSNEnv)
	}
	db, err := dao.InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	for _, table := range []string{"memory_facts", "memory_candidates"} {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clean table %s error = %v", table, err)
		}
	}
	repo := dao.NewMemoryRepo(db)
	return NewMemoryService(repo), repo
}

func TestIntegrationMemoryConsolidationMergesAndArchives(t *testing.T) {
	svc, repo := setupMemoryIntegration(t)
	ctx := context.Background()
	ownerID := int64(1001)
	first, err := svc.CreateMemory(ctx, CreateMemoryInput{
		BotID:          7,
		UserID:         ownerID,
		OwnerUserID:    ownerID,
		Scope:          model.ScopeUser,
		Type:           model.TypePreference,
		Content:        "用户喜欢简洁的回答",
		Source:         "test",
		Confidence:     0.8,
		Importance:     0.6,
		SourceEventIDs: []int64{11, 12},
	})
	if err != nil {
		t.Fatalf("CreateMemory(first) error = %v", err)
	}
	second, err := svc.CreateMemory(ctx, CreateMemoryInput{
		BotID:          7,
		UserID:         ownerID,
		OwnerUserID:    ownerID,
		Scope:          model.ScopeUser,
		Type:           model.TypePreference,
		Content:        "用户喜欢简洁的回答",
		Source:         "test",
		Confidence:     0.9,
		Importance:     0.9,
		SourceEventIDs: []int64{13},
	})
	if err != nil {
		t.Fatalf("CreateMemory(second) error = %v", err)
	}
	old := time.Now().AddDate(0, 0, -120)
	stale := &model.MemoryFact{
		BotID:          7,
		UserID:         1002,
		OwnerUserID:    1002,
		Scope:          model.ScopeUser,
		Type:           model.TypePreference,
		Content:        "旧的低重要度记忆",
		Visibility:     model.VisibilityPrivate,
		Enabled:        true,
		VectorStatus:   model.VectorDisabled,
		Confidence:     0.2,
		Importance:     0.05,
		LastUsedAt:     &old,
		SourceEventIDs: model.EncodeIDList([]int64{21}),
	}
	if err := repo.Create(ctx, stale); err != nil {
		t.Fatalf("repo.Create(stale) error = %v", err)
	}

	result, err := svc.ConsolidateMemories(ctx, ConsolidateInput{OwnerUserID: ownerID, Limit: 100})
	if err != nil {
		t.Fatalf("ConsolidateMemories() error = %v", err)
	}
	if result.DuplicatesMerged != 1 {
		t.Fatalf("duplicates merged = %d, want 1", result.DuplicatesMerged)
	}
	keeper, err := repo.GetByID(ctx, second.ID)
	if err != nil {
		t.Fatalf("GetByID(keeper) error = %v", err)
	}
	if keeper == nil || keeper.ReferenceCount != 0 {
		t.Fatalf("keeper = %#v", keeper)
	}
	sources := model.DecodeIDList(keeper.SourceEventIDs)
	if len(sources) != 3 || sources[0] != 11 || sources[1] != 12 || sources[2] != 13 {
		t.Fatalf("keeper source events = %#v, want [11 12 13]", sources)
	}
	duplicate, err := repo.GetByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetByID(duplicate) error = %v", err)
	}
	if duplicate.Enabled || duplicate.SupersededBy != second.ID {
		t.Fatalf("duplicate = %#v", duplicate)
	}

	archived, err := svc.ConsolidateMemories(ctx, ConsolidateInput{OwnerUserID: 1002, Limit: 100})
	if err != nil {
		t.Fatalf("ConsolidateMemories(stale) error = %v", err)
	}
	if archived.Archived != 1 {
		t.Fatalf("archived = %d, want 1", archived.Archived)
	}
	stored, err := repo.GetByID(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetByID(stale) error = %v", err)
	}
	if stored.Enabled || stored.ExpiredAt == nil {
		t.Fatalf("stale memory = %#v", stored)
	}
}

func TestIntegrationCandidateAcceptCarriesSourceEvents(t *testing.T) {
	svc, repo := setupMemoryIntegration(t)
	ctx := context.Background()
	ownerID := int64(1001)
	candidate, err := svc.CreateCandidate(ctx, CandidateInput{
		BotID:          7,
		UserID:         ownerID,
		OwnerUserID:    ownerID,
		Scope:          model.ScopeConversation,
		Type:           model.TypeProjectState,
		Content:        "项目当前处于 RAG 拆分阶段",
		Source:         "test",
		Evidence:       "event 31",
		Confidence:     0.8,
		Importance:     0.7,
		SourceEventIDs: []int64{31, 32},
	})
	if err != nil {
		t.Fatalf("CreateCandidate() error = %v", err)
	}
	accepted, err := svc.AcceptCandidate(ctx, ownerID, candidate.ID)
	if err != nil {
		t.Fatalf("AcceptCandidate() error = %v", err)
	}
	if accepted.AcceptedMemoryID == 0 {
		t.Fatalf("accepted = %#v", accepted)
	}
	fact, err := repo.GetByID(ctx, accepted.AcceptedMemoryID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	sources := model.DecodeIDList(fact.SourceEventIDs)
	if len(sources) != 2 || sources[0] != 31 || sources[1] != 32 {
		t.Fatalf("source events = %#v", sources)
	}
}

func TestIntegrationRecallIncrementsReferenceCount(t *testing.T) {
	svc, repo := setupMemoryIntegration(t)
	ctx := context.Background()
	ownerID := int64(1001)
	fact, err := svc.CreateMemory(ctx, CreateMemoryInput{
		BotID:          7,
		UserID:         ownerID,
		OwnerUserID:    ownerID,
		Scope:          model.ScopeUser,
		Type:           model.TypePreference,
		Content:        "ClaranAIM 的检索需要支持 RRF 融合和中文分词。",
		Source:         "test",
		Confidence:     0.9,
		Importance:     0.8,
		SourceEventIDs: []int64{41},
	})
	if err != nil {
		t.Fatalf("CreateMemory() error = %v", err)
	}
	result, err := svc.Recall(ctx, RecallInput{
		BotID:  7,
		UserID: ownerID,
		Limit:  5,
		Query:  "RRF 中文分词 检索",
	})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	found := false
	for _, item := range result.Facts {
		if item.ID == fact.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("fact %d not recalled: %#v", fact.ID, result.Facts)
	}
	stored, err := repo.GetByID(ctx, fact.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.ReferenceCount != 1 || stored.LastUsedAt == nil {
		t.Fatalf("reference count = %d last_used=%v", stored.ReferenceCount, stored.LastUsedAt)
	}
}
