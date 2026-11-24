package service

import (
	"ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/model"
	"ClaranAIM/pkg/observability"
	"context"
	"errors"
	"log"
	"sort"
	"strings"
	"time"
)

const (
	consolidationDefaultLimit = 1000
	consolidationStaleDays    = 90
	consolidationDecayFactor  = 0.8
	consolidationArchiveBelow = 0.05
)

// ConsolidateMemories 合并重复事实、衰减低重要度记忆并归档过期事实。
// 这是一个可重复执行的幂等任务，dry_run 只计算数量不落库。
func (s *memoryServiceImpl) ConsolidateMemories(ctx context.Context, input ConsolidateInput) (ConsolidateResult, error) {
	if input.OwnerUserID > 0 {
		return s.consolidateOwner(ctx, input.OwnerUserID, input.BotID, input.DryRun, input.Limit)
	}
	if s.repo == nil {
		return ConsolidateResult{}, errors.New("memory repository未配置")
	}
	owners, err := s.repo.ListOwnerUserIDs(ctx, input.BotID)
	if err != nil {
		return ConsolidateResult{}, err
	}
	total := ConsolidateResult{}
	for _, ownerID := range owners {
		result, err := s.consolidateOwner(ctx, ownerID, input.BotID, input.DryRun, input.Limit)
		if err != nil {
			return total, err
		}
		total.DuplicatesMerged += result.DuplicatesMerged
		total.ImportanceDecayed += result.ImportanceDecayed
		total.Archived += result.Archived
	}
	return total, nil
}

func (s *memoryServiceImpl) consolidateOwner(ctx context.Context, ownerID, botID int64, dryRun bool, requestedLimit int) (ConsolidateResult, error) {
	start := time.Now()
	status := "success"
	defer func() {
		observability.RecordBusinessDuration("memory", "consolidate", status, time.Since(start))
		observability.RecordBusinessEvent("memory", "consolidate", status)
	}()
	if s.repo == nil {
		status = "error"
		return ConsolidateResult{}, errors.New("memory repository未配置")
	}
	if ownerID <= 0 {
		status = "error"
		return ConsolidateResult{}, errors.New("owner_user_id不能为空")
	}
	limit := requestedLimit
	if limit <= 0 || limit > 5000 {
		limit = consolidationDefaultLimit
	}
	facts, _, err := s.repo.List(ctx, dao.MemoryFilter{
		OwnerUserID:     ownerID,
		BotID:           botID,
		IncludeDisabled: false,
		Limit:           limit,
	})
	if err != nil {
		status = "error"
		return ConsolidateResult{}, err
	}
	now := time.Now()
	byID := make(map[int64]*model.MemoryFact, len(facts))
	groups := make(map[string][]int64)
	for i := range facts {
		fact := &facts[i]
		byID[fact.ID] = fact
		if !fact.Enabled || fact.ExpiredAt != nil {
			continue
		}
		key := consolidationKey(*fact)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], fact.ID)
	}
	result := ConsolidateResult{}
	for _, ids := range groups {
		if len(ids) <= 1 {
			continue
		}
		sort.Slice(ids, func(i, j int) bool { return consolidationScore(byID[ids[i]]) > consolidationScore(byID[ids[j]]) })
		keeper := byID[ids[0]]
		mergedIDs := model.DecodeIDList(keeper.SourceEventIDs)
		for _, duplicateID := range ids[1:] {
			duplicate := byID[duplicateID]
			if duplicate == nil {
				continue
			}
			mergedIDs = mergeEventIDs(mergedIDs, model.DecodeIDList(duplicate.SourceEventIDs))
			keeper.ReferenceCount += duplicate.ReferenceCount
			if duplicate.Confidence > keeper.Confidence {
				keeper.Confidence = duplicate.Confidence
			}
			if duplicate.Importance > keeper.Importance {
				keeper.Importance = duplicate.Importance
			}
			result.DuplicatesMerged++
			if dryRun {
				continue
			}
			duplicate.Enabled = false
			duplicate.ExpiredAt = &now
			duplicate.SupersededBy = keeper.ID
			duplicate.PreviousMemoryID = keeper.ID
			if err := s.repo.Update(ctx, duplicate); err != nil {
				log.Printf("memory consolidation 合并失败 duplicate_id=%d err=%v", duplicate.ID, err)
			}
		}
		if dryRun {
			continue
		}
		keeper.SourceEventIDs = model.EncodeIDList(mergedIDs)
		if err := s.repo.Update(ctx, keeper); err != nil {
			log.Printf("memory consolidation 更新保留项失败 keep_id=%d err=%v", keeper.ID, err)
		}
	}
	staleBefore := now.AddDate(0, 0, -consolidationStaleDays)
	for i := range facts {
		fact := &facts[i]
		if !fact.Enabled || fact.ExpiredAt != nil {
			continue
		}
		if fact.LastUsedAt != nil && fact.LastUsedAt.After(staleBefore) {
			continue
		}
		nextImportance := fact.Importance * consolidationDecayFactor
		if nextImportance < consolidationArchiveBelow {
			result.Archived++
			if dryRun {
				continue
			}
			fact.Enabled = false
			fact.ExpiredAt = &now
			fact.Importance = nextImportance
			if err := s.repo.Update(ctx, fact); err != nil {
				log.Printf("memory consolidation 归档失败 memory_id=%d err=%v", fact.ID, err)
			}
			continue
		}
		if nextImportance >= fact.Importance {
			continue
		}
		result.ImportanceDecayed++
		if dryRun {
			continue
		}
		fact.Importance = nextImportance
		if err := s.repo.Update(ctx, fact); err != nil {
			log.Printf("memory consolidation 衰减失败 memory_id=%d err=%v", fact.ID, err)
		}
	}
	log.Printf("memory consolidation: owner=%d bot=%d dry_run=%t duplicates=%d decayed=%d archived=%d",
		ownerID, botID, dryRun, result.DuplicatesMerged, result.ImportanceDecayed, result.Archived)
	return result, nil
}

func consolidationKey(fact model.MemoryFact) string {
	content := strings.ToLower(strings.Join(strings.Fields(fact.Content), " "))
	if content == "" {
		return ""
	}
	return strings.ToLower(fact.Scope) + "|" + strings.ToLower(fact.Type) + "|" + content
}

func consolidationScore(fact *model.MemoryFact) float64 {
	if fact == nil {
		return 0
	}
	return float64(fact.ReferenceCount) + fact.Importance*10 + fact.Confidence
}

func mergeEventIDs(base, values []int64) []int64 {
	seen := make(map[int64]bool, len(base)+len(values))
	out := make([]int64, 0, len(base)+len(values))
	for _, id := range append(base, values...) {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
