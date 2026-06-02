package memoryclient

import (
	"ClaranAIM/kitex_gen/memory"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"context"
	"errors"
)

// RPCClient 用 Kitex 调用 memory-service。
// 它实现 Service 接口，让 api-gateway 和 agent-manager 不再依赖内部 HTTP transport。
type RPCClient struct {
	client memoryservice.Client
}

// NewRPCClient 包装已初始化的 memory-service Kitex 客户端。
func NewRPCClient(client memoryservice.Client) *RPCClient {
	return &RPCClient{client: client}
}

// CreateMemory 写入一条事实记忆。
func (c *RPCClient) CreateMemory(ctx context.Context, input CreateMemoryInput) (*MemoryFact, error) {
	resp, err := c.client.CreateMemory(ctx, &memory.CreateMemoryReq{
		BotId:            input.BotID,
		UserId:           input.UserID,
		OwnerUserId:      input.OwnerUserID,
		GroupId:          input.GroupID,
		ConversationId:   input.ConversationID,
		SessionId:        input.SessionID,
		Scope:            input.Scope,
		Type:             input.Type,
		Title:            input.Title,
		Content:          input.Content,
		Source:           input.Source,
		Visibility:       input.Visibility,
		Enabled:          boolValue(input.Enabled, true),
		EnabledSet:       input.Enabled != nil,
		VectorStatus:     input.VectorStatus,
		EmbeddingRef:     input.EmbeddingRef,
		Confidence:       input.Confidence,
		Importance:       input.Importance,
		PreviousMemoryId: input.PreviousMemoryID,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCMemoryFact(resp.GetMemory()), nil
}

// ListMemories 按 viewerID 的权限视角查询记忆。
func (c *RPCClient) ListMemories(ctx context.Context, viewerID int64, filter Filter) ([]MemoryFact, int64, error) {
	resp, err := c.client.ListMemories(ctx, &memory.ListMemoriesReq{
		ViewerId: viewerID,
		Filter: &memory.MemoryFilter{
			BotId:           filter.BotID,
			UserId:          filter.UserID,
			OwnerUserId:     filter.OwnerUserID,
			GroupId:         filter.GroupID,
			ConversationId:  filter.ConversationID,
			SessionId:       filter.SessionID,
			Scopes:          filter.Scopes,
			Types:           filter.Types,
			IncludeDisabled: filter.IncludeDisabled,
			Limit:           int64(filter.Limit),
			Offset:          int64(filter.Offset),
		},
	})
	if err != nil {
		return nil, 0, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, 0, err
	}
	return fromRPCMemoryFacts(resp.GetMemories()), resp.GetTotal(), nil
}

// Recall 为 Agent 上下文构建召回候选记忆。
func (c *RPCClient) Recall(ctx context.Context, input RecallInput) (RecallResult, error) {
	resp, err := c.client.Recall(ctx, &memory.RecallReq{
		BotId:            input.BotID,
		UserId:           input.UserID,
		GroupId:          input.GroupID,
		ConversationId:   input.ConversationID,
		SessionId:        input.SessionID,
		Limit:            int64(input.Limit),
		Query:            input.Query,
		MinScore:         input.MinScore,
		VectorCandidateK: int64(input.VectorCandidateK),
		UseLlmFilter:     input.UseLLMFilter,
	})
	if err != nil {
		return RecallResult{}, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return RecallResult{}, err
	}
	return RecallResult{Facts: fromRPCMemoryFacts(resp.GetFacts()), ContextText: resp.GetContextText()}, nil
}

// UpdateMemory 修改当前用户拥有的记忆。
func (c *RPCClient) UpdateMemory(ctx context.Context, viewerID, memoryID int64, input UpdateMemoryInput) (*MemoryFact, error) {
	resp, err := c.client.UpdateMemory(ctx, &memory.UpdateMemoryReq{
		ViewerId:      viewerID,
		MemoryId:      memoryID,
		Scope:         input.Scope,
		Type:          input.Type,
		Title:         input.Title,
		Content:       input.Content,
		Source:        input.Source,
		Visibility:    input.Visibility,
		Enabled:       boolValue(input.Enabled, true),
		EnabledSet:    input.Enabled != nil,
		VectorStatus:  input.VectorStatus,
		EmbeddingRef:  input.EmbeddingRef,
		Confidence:    float64Value(input.Confidence),
		ConfidenceSet: input.Confidence != nil,
		Importance:    float64Value(input.Importance),
		ImportanceSet: input.Importance != nil,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCMemoryFact(resp.GetMemory()), nil
}

func (c *RPCClient) CreateCandidate(ctx context.Context, input CandidateInput) (*MemoryCandidate, error) {
	resp, err := c.client.CreateCandidate(ctx, &memory.CreateCandidateReq{
		BotId:              input.BotID,
		UserId:             input.UserID,
		OwnerUserId:        input.OwnerUserID,
		GroupId:            input.GroupID,
		ConversationId:     input.ConversationID,
		SessionId:          input.SessionID,
		Scope:              input.Scope,
		Type:               input.Type,
		Title:              input.Title,
		Content:            input.Content,
		Source:             input.Source,
		Evidence:           input.Evidence,
		Confidence:         input.Confidence,
		Importance:         input.Importance,
		ConflictMemoryIds:  input.ConflictMemoryIDs,
		ConflictResolution: input.ConflictResolution,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCCandidate(resp.GetCandidate()), nil
}

func (c *RPCClient) ListCandidates(ctx context.Context, viewerID int64, filter CandidateFilter) ([]MemoryCandidate, int64, error) {
	resp, err := c.client.ListCandidates(ctx, &memory.ListCandidatesReq{
		ViewerId: viewerID,
		Filter: &memory.CandidateFilter{
			BotId:  filter.BotID,
			UserId: filter.UserID,
			Status: filter.Status,
			Limit:  int64(filter.Limit),
			Offset: int64(filter.Offset),
		},
	})
	if err != nil {
		return nil, 0, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, 0, err
	}
	return fromRPCCandidates(resp.GetCandidates()), resp.GetTotal(), nil
}

func (c *RPCClient) AcceptCandidate(ctx context.Context, viewerID, candidateID int64) (*MemoryCandidate, error) {
	resp, err := c.client.AcceptCandidate(ctx, &memory.CandidateActionReq{ViewerId: viewerID, CandidateId: candidateID})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCCandidate(resp.GetCandidate()), nil
}

func (c *RPCClient) RejectCandidate(ctx context.Context, viewerID, candidateID int64) (*MemoryCandidate, error) {
	resp, err := c.client.RejectCandidate(ctx, &memory.CandidateActionReq{ViewerId: viewerID, CandidateId: candidateID})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCCandidate(resp.GetCandidate()), nil
}

// DeleteMemory 删除或关闭一条记忆。
func (c *RPCClient) DeleteMemory(ctx context.Context, viewerID, memoryID int64) error {
	resp, err := c.client.DeleteMemory(ctx, &memory.DeleteMemoryReq{ViewerId: viewerID, MemoryId: memoryID})
	if err != nil {
		return err
	}
	return rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

func fromRPCMemoryFacts(facts []*memory.MemoryFact) []MemoryFact {
	out := make([]MemoryFact, 0, len(facts))
	for _, fact := range facts {
		item := fromRPCMemoryFact(fact)
		if item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func fromRPCMemoryFact(fact *memory.MemoryFact) *MemoryFact {
	if fact == nil {
		return nil
	}
	return &MemoryFact{
		ID:               fact.GetId(),
		BotID:            fact.GetBotId(),
		UserID:           fact.GetUserId(),
		OwnerUserID:      fact.GetOwnerUserId(),
		GroupID:          fact.GetGroupId(),
		ConversationID:   fact.GetConversationId(),
		SessionID:        fact.GetSessionId(),
		Scope:            fact.GetScope(),
		Type:             fact.GetType(),
		Title:            fact.GetTitle(),
		Content:          fact.GetContent(),
		Source:           fact.GetSource(),
		Visibility:       fact.GetVisibility(),
		Enabled:          fact.GetEnabled(),
		VectorStatus:     fact.GetVectorStatus(),
		EmbeddingRef:     fact.GetEmbeddingRef(),
		Confidence:       fact.GetConfidence(),
		Importance:       fact.GetImportance(),
		VectorScore:      fact.GetVectorScore(),
		FinalScore:       fact.GetFinalScore(),
		ScoreReason:      fact.GetScoreReason(),
		ExpiredAt:        fact.GetExpiredAt(),
		SupersededBy:     fact.GetSupersededBy(),
		PreviousMemoryID: fact.GetPreviousMemoryId(),
		CreatedAt:        fact.GetCreatedAt(),
		UpdatedAt:        fact.GetUpdatedAt(),
	}
}

func fromRPCCandidates(candidates []*memory.MemoryCandidate) []MemoryCandidate {
	out := make([]MemoryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		item := fromRPCCandidate(candidate)
		if item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func fromRPCCandidate(candidate *memory.MemoryCandidate) *MemoryCandidate {
	if candidate == nil {
		return nil
	}
	return &MemoryCandidate{
		ID:                 candidate.GetId(),
		BotID:              candidate.GetBotId(),
		UserID:             candidate.GetUserId(),
		OwnerUserID:        candidate.GetOwnerUserId(),
		GroupID:            candidate.GetGroupId(),
		ConversationID:     candidate.GetConversationId(),
		SessionID:          candidate.GetSessionId(),
		Scope:              candidate.GetScope(),
		Type:               candidate.GetType(),
		Title:              candidate.GetTitle(),
		Content:            candidate.GetContent(),
		Source:             candidate.GetSource(),
		Evidence:           candidate.GetEvidence(),
		Confidence:         candidate.GetConfidence(),
		Importance:         candidate.GetImportance(),
		Status:             candidate.GetStatus(),
		ConflictMemoryIDs:  candidate.GetConflictMemoryIds(),
		ConflictResolution: candidate.GetConflictResolution(),
		AcceptedMemoryID:   candidate.GetAcceptedMemoryId(),
		CreatedAt:          candidate.GetCreatedAt(),
		UpdatedAt:          candidate.GetUpdatedAt(),
	}
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func float64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func rpcStatus(success bool, msg string) error {
	if success {
		return nil
	}
	if msg == "" {
		msg = "memory-service RPC调用失败"
	}
	return errors.New(msg)
}
