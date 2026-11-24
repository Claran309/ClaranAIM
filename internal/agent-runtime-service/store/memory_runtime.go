package store

import (
	"context"
	"sort"
	"sync"
)

// MemoryRuntimeStore 在无 MySQL 的本地开发/单元测试中提供运行、审批和 checkpoint 存储。
// 它不提供跨进程持久化，生产路径必须使用 GormStore。
type MemoryRuntimeStore struct {
	mu          sync.RWMutex
	runs        map[string]RunInfo
	checkpoints map[string][]byte
}

// NewMemoryRuntimeStore 创建内存运行时存储。
func NewMemoryRuntimeStore() *MemoryRuntimeStore {
	return &MemoryRuntimeStore{
		runs:        make(map[string]RunInfo),
		checkpoints: make(map[string][]byte),
	}
}

// CreateRun 保存运行记录。
func (s *MemoryRuntimeStore) CreateRun(ctx context.Context, run *RunInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.RunID] = *run
	return nil
}

// UpdateRun 更新运行记录。
func (s *MemoryRuntimeStore) UpdateRun(ctx context.Context, run *RunInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.RunID] = *run
	return nil
}

// GetRun 查询运行记录。
func (s *MemoryRuntimeStore) GetRun(ctx context.Context, runID string) (*RunInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, nil
	}
	return &run, nil
}

// ListRuns 返回过滤后的运行记录。
func (s *MemoryRuntimeStore) ListRuns(ctx context.Context, filter RunFilter) ([]RunInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RunInfo, 0, len(s.runs))
	for _, run := range s.runs {
		if filter.UserID > 0 && run.UserID != filter.UserID {
			continue
		}
		if filter.BotID > 0 && run.BotID != filter.BotID {
			continue
		}
		if filter.ConversationID > 0 && run.ConversationID != filter.ConversationID {
			continue
		}
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Get 读取 checkpoint。
func (s *MemoryRuntimeStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.checkpoints[checkPointID]
	if !ok {
		return nil, false, nil
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, true, nil
}

// Set 写入 checkpoint。
func (s *MemoryRuntimeStore) Set(ctx context.Context, checkPointID string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, len(data))
	copy(out, data)
	s.checkpoints[checkPointID] = out
	return nil
}
