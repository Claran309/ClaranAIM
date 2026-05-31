package memoryclient

import "context"

// 这些枚举值同时写入 memory_facts 表并暴露给网关/Agent 服务。
// Scope 决定记忆隔离边界，Visibility 决定谁能读取，VectorStatus 为后续向量化召回预留状态位。
const (
	ScopeUser         = "user"
	ScopeGroup        = "group"
	ScopeConversation = "conversation"
	ScopeSession      = "session"

	VisibilityPrivate = "private"
	VisibilityShared  = "shared"

	TypePreference    = "preference"
	TypeSpeakingStyle = "speaking_style"
	TypeLongTermGoal  = "long_term_goal"
	TypeGroupProfile  = "group_profile"
	TypeProjectState  = "project_state"
	TypeChatSummary   = "chat_summary"
	TypeAgentRun      = "agent_run_summary"

	VectorPending  = "pending"
	VectorDisabled = "disabled"
	VectorReady    = "ready"
)

// MemoryFact 是跨服务传输的记忆事实 DTO。
// 它刻意放在 pkg 中，供 api-gateway、agent-manager-service 和 memory-service 共享接口契约，
// 避免微服务之间直接 import 对方 internal 包。
type MemoryFact struct {
	ID             int64   `json:"id"`
	BotID          int64   `json:"bot_id"`
	UserID         int64   `json:"user_id"`
	OwnerUserID    int64   `json:"owner_user_id"`
	GroupID        int64   `json:"group_id"`
	ConversationID int64   `json:"conversation_id"`
	SessionID      string  `json:"session_id"`
	Scope          string  `json:"scope"`
	Type           string  `json:"type"`
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	Source         string  `json:"source"`
	Visibility     string  `json:"visibility"`
	Enabled        bool    `json:"enabled"`
	VectorStatus   string  `json:"vector_status"`
	EmbeddingRef   string  `json:"embedding_ref"`
	Confidence     float64 `json:"confidence"`
	CreatedAt      string  `json:"created_at,omitempty"`
	UpdatedAt      string  `json:"updated_at,omitempty"`
}

// Filter 表示记忆列表查询条件。
type Filter struct {
	BotID           int64
	UserID          int64
	OwnerUserID     int64
	GroupID         int64
	ConversationID  int64
	SessionID       string
	Scopes          []string
	Types           []string
	IncludeDisabled bool
	Limit           int
	Offset          int
}

// CreateMemoryInput 表示创建记忆事实的跨服务入参。
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

// UpdateMemoryInput 表示用户可治理记忆的更新入参。
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

// RecallInput 表示 Agent 召回长期记忆时的上下文边界。
type RecallInput struct {
	BotID          int64
	UserID         int64
	GroupID        int64
	ConversationID int64
	SessionID      string
	Limit          int
}

// RecallResult 表示召回到的记忆事实和可直接注入 prompt 的文本。
type RecallResult struct {
	Facts       []MemoryFact `json:"facts"`
	ContextText string       `json:"context_text"`
}

// Service 是 memory-service 对其他服务暴露的最小客户端契约。
type Service interface {
	CreateMemory(ctx context.Context, input CreateMemoryInput) (*MemoryFact, error)
	ListMemories(ctx context.Context, viewerID int64, filter Filter) ([]MemoryFact, int64, error)
	Recall(ctx context.Context, input RecallInput) (RecallResult, error)
	UpdateMemory(ctx context.Context, viewerID, memoryID int64, input UpdateMemoryInput) (*MemoryFact, error)
	DeleteMemory(ctx context.Context, viewerID, memoryID int64) error
}
