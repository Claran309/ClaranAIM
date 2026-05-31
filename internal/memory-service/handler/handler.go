// Package handler 实现 memory-service 的 Kitex RPC 入口。
package handler

import (
	memorydao "ClaranAIM/internal/memory-service/dao"
	memorysvc "ClaranAIM/internal/memory-service/service"
	"ClaranAIM/kitex_gen/memory"
	"ClaranAIM/pkg/memoryclient"
	"context"
)

// MemoryServiceImpl 将 Kitex RPC 请求转交给 memory service。
// 权限裁剪和记忆隔离仍由 service 层统一负责，handler 不直接访问 DAO。
type MemoryServiceImpl struct {
	svc memorysvc.MemoryService
}

// NewMemoryServiceImpl 创建 memory-service 的 Kitex handler。
func NewMemoryServiceImpl(svc memorysvc.MemoryService) memory.MemoryService {
	return &MemoryServiceImpl{svc: svc}
}

// CreateMemory 创建一条可治理的记忆事实。
func (h *MemoryServiceImpl) CreateMemory(ctx context.Context, req *memory.CreateMemoryReq) (*memory.CreateMemoryResp, error) {
	fact, err := h.svc.CreateMemory(ctx, memoryclient.CreateMemoryInput{
		BotID:          req.BotId,
		UserID:         req.UserId,
		OwnerUserID:    req.OwnerUserId,
		GroupID:        req.GroupId,
		ConversationID: req.ConversationId,
		SessionID:      req.SessionId,
		Scope:          req.Scope,
		Type:           req.Type,
		Title:          req.Title,
		Content:        req.Content,
		Source:         req.Source,
		Visibility:     req.Visibility,
		Enabled:        optionalBool(req.Enabled, req.EnabledSet),
		VectorStatus:   req.VectorStatus,
		EmbeddingRef:   req.EmbeddingRef,
		Confidence:     req.Confidence,
	})
	if err != nil {
		return &memory.CreateMemoryResp{Success: false, Msg: err.Error()}, nil
	}
	return &memory.CreateMemoryResp{Success: true, Memory: toRPCMemoryFact(fact)}, nil
}

// ListMemories 查询当前用户可见且可管理的记忆列表。
func (h *MemoryServiceImpl) ListMemories(ctx context.Context, req *memory.ListMemoriesReq) (*memory.ListMemoriesResp, error) {
	facts, total, err := h.svc.ListMemories(ctx, req.ViewerId, toDAOFilter(req.Filter))
	if err != nil {
		return &memory.ListMemoriesResp{Success: false, Msg: err.Error()}, nil
	}
	return &memory.ListMemoriesResp{Success: true, Memories: toRPCMemoryFacts(facts), Total: total}, nil
}

// Recall 为 Agent 上下文构建召回长期记忆。
func (h *MemoryServiceImpl) Recall(ctx context.Context, req *memory.RecallReq) (*memory.RecallResp, error) {
	result, err := h.svc.Recall(ctx, memoryclient.RecallInput{
		BotID:          req.BotId,
		UserID:         req.UserId,
		GroupID:        req.GroupId,
		ConversationID: req.ConversationId,
		SessionID:      req.SessionId,
		Limit:          int(req.Limit),
	})
	if err != nil {
		return &memory.RecallResp{Success: false, Msg: err.Error()}, nil
	}
	return &memory.RecallResp{Success: true, Facts: toRPCMemoryFacts(result.Facts), ContextText: result.ContextText}, nil
}

// UpdateMemory 修改当前用户拥有的记忆事实。
func (h *MemoryServiceImpl) UpdateMemory(ctx context.Context, req *memory.UpdateMemoryReq) (*memory.UpdateMemoryResp, error) {
	fact, err := h.svc.UpdateMemory(ctx, req.ViewerId, req.MemoryId, memoryclient.UpdateMemoryInput{
		Scope:        req.Scope,
		Type:         req.Type,
		Title:        req.Title,
		Content:      req.Content,
		Source:       req.Source,
		Visibility:   req.Visibility,
		Enabled:      optionalBool(req.Enabled, req.EnabledSet),
		VectorStatus: req.VectorStatus,
		EmbeddingRef: req.EmbeddingRef,
		Confidence:   optionalFloat64(req.Confidence, req.ConfidenceSet),
	})
	if err != nil {
		return &memory.UpdateMemoryResp{Success: false, Msg: err.Error()}, nil
	}
	return &memory.UpdateMemoryResp{Success: true, Memory: toRPCMemoryFact(fact)}, nil
}

// DeleteMemory 删除当前用户拥有的记忆事实。
func (h *MemoryServiceImpl) DeleteMemory(ctx context.Context, req *memory.DeleteMemoryReq) (*memory.DeleteMemoryResp, error) {
	if err := h.svc.DeleteMemory(ctx, req.ViewerId, req.MemoryId); err != nil {
		return &memory.DeleteMemoryResp{Success: false, Msg: err.Error()}, nil
	}
	return &memory.DeleteMemoryResp{Success: true, Msg: "删除成功"}, nil
}

func toDAOFilter(filter *memory.MemoryFilter) memorydao.MemoryFilter {
	if filter == nil {
		return memorydao.MemoryFilter{}
	}
	return memorydao.MemoryFilter{
		BotID:           filter.BotId,
		UserID:          filter.UserId,
		OwnerUserID:     filter.OwnerUserId,
		GroupID:         filter.GroupId,
		ConversationID:  filter.ConversationId,
		SessionID:       filter.SessionId,
		Scopes:          filter.Scopes,
		Types:           filter.Types,
		IncludeDisabled: filter.IncludeDisabled,
		Limit:           int(filter.Limit),
		Offset:          int(filter.Offset),
	}
}

func toRPCMemoryFacts(facts []memoryclient.MemoryFact) []*memory.MemoryFact {
	out := make([]*memory.MemoryFact, 0, len(facts))
	for i := range facts {
		out = append(out, toRPCMemoryFact(&facts[i]))
	}
	return out
}

func toRPCMemoryFact(fact *memoryclient.MemoryFact) *memory.MemoryFact {
	if fact == nil {
		return nil
	}
	return &memory.MemoryFact{
		Id:             fact.ID,
		BotId:          fact.BotID,
		UserId:         fact.UserID,
		OwnerUserId:    fact.OwnerUserID,
		GroupId:        fact.GroupID,
		ConversationId: fact.ConversationID,
		SessionId:      fact.SessionID,
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
		CreatedAt:      fact.CreatedAt,
		UpdatedAt:      fact.UpdatedAt,
	}
}

func optionalBool(value bool, set bool) *bool {
	if !set {
		return nil
	}
	return &value
}

func optionalFloat64(value float64, set bool) *float64 {
	if !set {
		return nil
	}
	return &value
}
