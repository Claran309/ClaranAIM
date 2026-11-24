package store

import (
	"context"
	"time"
)

const (
	// DefaultContextLimit 是每次送入模型的最近会话消息条数。
	DefaultContextLimit = 12
	// DefaultSummaryTrigger 是触发滚动摘要的消息总数阈值。
	DefaultSummaryTrigger = 24
	// DefaultSummaryMaxRunes 是滚动摘要的最大字符数，防止摘要本身无限增长。
	DefaultSummaryMaxRunes = 4000
)

// 运行状态常量。
const (
	RunStatusRunning         = "running"
	RunStatusCompleted       = "completed"
	RunStatusFailed          = "failed"
	RunStatusPendingApproval = "pending_approval"
	RunStatusBudgetExceeded  = "budget_exceeded"
	RunStatusInterrupted     = "interrupted"
)

// Message 是存储层与运行时之间的消息视图。
type Message struct {
	ID         int64
	Role       string
	Content    string
	TokenCount int64
	CreatedAt  time.Time
}

// Session 是会话及其消息的聚合。
type Session struct {
	ID        string
	Title     string
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
	Messages  []Message
}

// SessionMeta 是会话列表返回的轻量元数据。
type SessionMeta struct {
	ID        string
	Title     string
	Summary   string
	CreatedAt time.Time
}

// SessionOwner 标识会话归属，防止不同 Agent/用户/会话串数据。
type SessionOwner struct {
	BotID          int64
	UserID         int64
	ConversationID int64
}

// RunInfo 是一次 Agent 运行的完整状态。
type RunInfo struct {
	RunID           string
	SessionID       string
	BotID           int64
	UserID          int64
	ConversationID  int64
	Status          string
	Step            int64
	InputTokens     int64
	OutputTokens    int64
	CostUSD         float64
	InterruptReason string
	ApprovalID      string
	InterruptedTool string
	CheckpointJSON  string
	ReflectionJSON  string
	StartedAt       time.Time
	FinishedAt      *time.Time
}

// RunFilter 是运行记录的查询条件。
type RunFilter struct {
	UserID         int64
	BotID          int64
	ConversationID int64
	Limit          int
}

// SessionStore 抽象会话持久化，生产使用 MySQL，测试和本地开发可回退 JSONL。
type SessionStore interface {
	GetSession(ctx context.Context, id string) (*Session, error)
	AppendMessage(ctx context.Context, id string, owner SessionOwner, role, content string, tokenCount int64) error
	Context(ctx context.Context, id string, limit int) (string, []Message, error)
	ListSessions(ctx context.Context, owner SessionOwner) ([]SessionMeta, error)
	DeleteSession(ctx context.Context, id string) error
}

// RunStore 抽象运行记录持久化。
type RunStore interface {
	CreateRun(ctx context.Context, run *RunInfo) error
	UpdateRun(ctx context.Context, run *RunInfo) error
	GetRun(ctx context.Context, runID string) (*RunInfo, error)
	ListRuns(ctx context.Context, filter RunFilter) ([]RunInfo, error)
}

// CheckpointStore 是 Eino Runner 的持久化检查点接口。
type CheckpointStore interface {
	Get(ctx context.Context, checkPointID string) ([]byte, bool, error)
	Set(ctx context.Context, checkPointID string, data []byte) error
}
