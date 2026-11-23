package store

import "time"

// AgentSession 保存 Agent 长会话的元数据、滚动摘要和最近一次更新时间。
type AgentSession struct {
	ID                string    `gorm:"column:id;type:varchar(128);primaryKey"`
	BotID             int64     `gorm:"column:bot_id;index:idx_agent_session_owner;not null;default:0"`
	UserID            int64     `gorm:"column:user_id;index:idx_agent_session_owner;not null;default:0"`
	ConversationID    int64     `gorm:"column:conversation_id;index:idx_agent_session_owner;not null;default:0"`
	Title             string    `gorm:"column:title;type:varchar(255);not null;default:''"`
	Summary           string    `gorm:"column:summary;type:text"`
	SummarizedThrough int64     `gorm:"column:summarized_through;not null;default:0"`
	MessageCount      int64     `gorm:"column:message_count;not null;default:0"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 固定会话表名，便于跨服务查询和运维。
func (AgentSession) TableName() string { return "agent_sessions" }

// AgentMessage 保存一条会话消息；正文只落库，不写日志。
type AgentMessage struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID  string    `gorm:"column:session_id;type:varchar(128);index:idx_agent_message_session;not null"`
	Role       string    `gorm:"column:role;type:varchar(32);not null"`
	Content    string    `gorm:"column:content;type:mediumtext"`
	TokenCount int64     `gorm:"column:token_count;not null;default:0"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 固定消息表名。
func (AgentMessage) TableName() string { return "agent_messages" }

// AgentRun 保存一次 Agent 运行的预算、审批、token 和成本状态。
type AgentRun struct {
	RunID           string     `gorm:"column:run_id;type:varchar(64);primaryKey"`
	SessionID       string     `gorm:"column:session_id;type:varchar(128);index:idx_agent_run_session;not null"`
	BotID           int64      `gorm:"column:bot_id;index:idx_agent_run_owner;not null;default:0"`
	UserID          int64      `gorm:"column:user_id;index:idx_agent_run_owner;not null;default:0"`
	ConversationID  int64      `gorm:"column:conversation_id;index:idx_agent_run_owner;not null;default:0"`
	Status          string     `gorm:"column:status;type:varchar(32);not null;default:'running'"`
	Step            int64      `gorm:"column:step;not null;default:0"`
	InputTokens     int64      `gorm:"column:input_tokens;not null;default:0"`
	OutputTokens    int64      `gorm:"column:output_tokens;not null;default:0"`
	CostUSD         float64    `gorm:"column:cost_usd;not null;default:0"`
	InterruptReason string     `gorm:"column:interrupt_reason;type:varchar(255);not null;default:''"`
	ApprovalID      string     `gorm:"column:approval_id;type:varchar(64);index;not null;default:''"`
	InterruptedTool string     `gorm:"column:interrupted_tool;type:varchar(128);not null;default:''"`
	CheckpointJSON  string     `gorm:"column:checkpoint_json;type:mediumtext"`
	ReflectionJSON  string     `gorm:"column:reflection_json;type:mediumtext"`
	StartedAt       time.Time  `gorm:"column:started_at;autoCreateTime"`
	FinishedAt      *time.Time `gorm:"column:finished_at"`
}

// TableName 固定运行记录表名。
func (AgentRun) TableName() string { return "agent_runs" }

// AgentCheckpoint 保存 Eino Runner 的中断检查点，使不同 runtime 实例可以恢复同一运行。
type AgentCheckpoint struct {
	CheckPointID string    `gorm:"column:checkpoint_id;type:varchar(64);primaryKey"`
	Data         []byte    `gorm:"column:data;type:longblob;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 固定检查点表名。
func (AgentCheckpoint) TableName() string { return "agent_checkpoints" }
