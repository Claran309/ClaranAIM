// Package model 定义 agent-manager-service 的数据库模型。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// Bot 保存构建和运行一个 Agent 实例所需的配置。
// 表名暂时仍为 bots，是为了兼容已有数据库和历史 Kitex IDL；业务语义已经是 Agent。
type Bot struct {
	ID            int64  `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Name          string `json:"name" gorm:"size:100;not null"`
	Type          string `json:"type" gorm:"size:20;not null;default:internal"`
	Description   string `json:"description" gorm:"type:text"`
	ModelName     string `json:"model_name" gorm:"size:100;not null"`
	APIKey        string `json:"api_key" gorm:"size:255"`
	BaseURL       string `json:"base_url" gorm:"size:255"`
	SystemPrompt  string `json:"system_prompt" gorm:"type:text"`
	SkillsDir     string `json:"skills_dir" gorm:"size:255"`
	AgentRoot     string `json:"agent_root" gorm:"size:255"`
	AgentUserID   int64  `json:"agent_user_id" gorm:"index;default:0"`
	Avatar        string `json:"avatar" gorm:"size:255"`
	Signature     string `json:"signature" gorm:"size:120"`
	WorkspaceRoot string `json:"workspace_root" gorm:"size:255"`
	ToolPolicy    string `json:"tool_policy" gorm:"size:50;default:safe"`
	// ContextMessageLimit 控制上下文型任务从 msg-core-service 读取的最近消息条数。
	// 这是 Agent 会话感知能力的核心运行参数，前端可配置，服务端会做范围裁剪。
	ContextMessageLimit int64 `json:"context_message_limit" gorm:"default:80"`
	// MemoryRecallLimit 控制每轮 Agent 对话从 memory-service 召回的长期记忆条数。
	MemoryRecallLimit int64 `json:"memory_recall_limit" gorm:"default:12"`
	// MaxOutputTokens 预留给支持输出长度控制的模型配置；0 表示使用模型供应商默认值。
	MaxOutputTokens int64 `json:"max_output_tokens" gorm:"default:0"`
	// Temperature 预留给模型采样温度；0 表示使用运行时默认值，避免破坏旧配置。
	Temperature float64 `json:"temperature" gorm:"default:0"`
	// GroupTriggerMode 控制 Agent 在群聊中的默认触发方式，例如 mention、keyword、all。
	GroupTriggerMode string `json:"group_trigger_mode" gorm:"size:30;default:mention"`
	// AutoReplyEnabled 控制 Agent 是否允许根据订阅规则自动回复；关闭后仍可手动运行。
	AutoReplyEnabled bool      `json:"auto_reply_enabled" gorm:"default:true"`
	OwnerID          int64     `json:"owner_id" gorm:"index;not null"`
	IsActive         bool      `json:"is_active" gorm:"default:true"`
	CreatedAt        time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate 在插入 Agent 配置前补充分布式雪花 ID。
func (b *Bot) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&b.ID)
}

// TableName 固定历史表名 bots，避免重命名服务时误建新表导致数据丢失。
func (Bot) TableName() string {
	return "bots"
}

// BotPermission 记录非创建者用户对某个 Agent 的协作权限。
// owner/admin/operator/viewer 的实际能力由 service 层的 roleRank 和 requireRole 解释。
type BotPermission struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	BotID     int64     `json:"bot_id" gorm:"uniqueIndex:idx_bot_permission;not null"`
	UserID    int64     `json:"user_id" gorm:"uniqueIndex:idx_bot_permission;index;not null"`
	Role      string    `json:"role" gorm:"size:20;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// AgentDispatchRecord 记录一次由 Kafka 事件触发的 Agent 调度。
// event_id + agent_user_id 唯一索引用于保证幂等：Kafka 崩溃重投同一事件时，
// 已完成的 Agent 调度不会再次调用 runtime。Agent 回复也使用同源 key 作为 msg-core-service 的 client_msg_id，
// 用于覆盖“回复已落库但调度记录尚未标记 completed”的更窄崩溃窗口。
type AgentDispatchRecord struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	EventID        string    `json:"event_id" gorm:"size:64;uniqueIndex:uk_agent_dispatch_event_agent;not null"`
	AgentUserID    int64     `json:"agent_user_id" gorm:"uniqueIndex:uk_agent_dispatch_event_agent;index;not null"`
	BotID          int64     `json:"bot_id" gorm:"index;not null"`
	EventType      string    `json:"event_type" gorm:"size:50;index"`
	Decision       string    `json:"decision" gorm:"size:20;default:trigger"`
	SourceEventID  string    `json:"source_event_id" gorm:"size:80;index"`
	AgentTraceID   string    `json:"agent_trace_id" gorm:"size:80;index"`
	SourceMsgID    int64     `json:"source_msg_id" gorm:"index;not null"`
	ReplyMsgID     int64     `json:"reply_msg_id" gorm:"default:0"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`
	SenderID       int64     `json:"sender_id" gorm:"index;not null"`
	Status         string    `json:"status" gorm:"size:20;not null;default:started"`
	ErrorMessage   string    `json:"error_message" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate 在插入调度记录前补充分布式雪花 ID。
func (r *AgentDispatchRecord) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName 固定 Agent 调度记录表名。
func (AgentDispatchRecord) TableName() string {
	return "agent_dispatch_records"
}

// AgentSubscriptionRule 控制 Agent 何时观察、记录或响应 IM 原生事件。
// 群聊默认低打扰，只在 @、命令或显式规则命中时行动；规则也可以配置关键词、文件事件或静默入库。
type AgentSubscriptionRule struct {
	ID               int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	BotID            int64     `json:"bot_id" gorm:"index;not null"`
	AgentUserID      int64     `json:"agent_user_id" gorm:"index;not null"`
	SourceRouteID    int64     `json:"source_route_id" gorm:"uniqueIndex:uk_agent_subscription_route;default:0"`
	ConversationID   int64     `json:"conversation_id" gorm:"index;default:0"`
	ConversationType string    `json:"conversation_type" gorm:"size:20"`
	EventTypes       string    `json:"event_types" gorm:"size:255"`
	Keywords         string    `json:"keywords" gorm:"size:255"`
	CommandPrefix    string    `json:"command_prefix" gorm:"size:50"`
	TriggerMode      string    `json:"trigger_mode" gorm:"size:30;default:mention"`
	Action           string    `json:"action" gorm:"size:20;default:trigger"`
	Silent           bool      `json:"silent" gorm:"default:false"`
	IsActive         bool      `json:"is_active" gorm:"default:true"`
	CreatedAt        time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate 在插入订阅规则前补充分布式雪花 ID。
func (r *AgentSubscriptionRule) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName 固定 Agent 订阅规则表名。
func (AgentSubscriptionRule) TableName() string {
	return "agent_subscription_rules"
}

// AgentAuditRecord 是 Agent-Native 决策链路的持久化审计记录。
// 它会记录忽略、静默记录、触发执行等决策，使生产问题排查不必重新回放 Kafka payload。
type AgentAuditRecord struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	EventID        string    `json:"event_id" gorm:"size:80;index;not null"`
	EventType      string    `json:"event_type" gorm:"size:50;index"`
	BotID          int64     `json:"bot_id" gorm:"index;default:0"`
	AgentUserID    int64     `json:"agent_user_id" gorm:"index;default:0"`
	ConversationID int64     `json:"conversation_id" gorm:"index;default:0"`
	SenderID       int64     `json:"sender_id" gorm:"index;default:0"`
	Decision       string    `json:"decision" gorm:"size:20;index;not null"`
	Reason         string    `json:"reason" gorm:"size:255"`
	TraceID        string    `json:"trace_id" gorm:"size:80;index"`
	SourceMsgID    int64     `json:"source_msg_id" gorm:"index;default:0"`
	Metadata       string    `json:"metadata" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// BeforeCreate 在插入审计记录前补充分布式雪花 ID。
func (r *AgentAuditRecord) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName 固定 Agent 审计记录表名。
func (AgentAuditRecord) TableName() string {
	return "agent_audit_records"
}

// AgentTask 记录一次 Agent 工作任务的生命周期。
// 当前事件 consumer 仍同步调用 runtime，但会先写 task 状态；后续可平滑改成 runtime 异步领取任务。
type AgentTask struct {
	ID             int64      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	BotID          int64      `json:"bot_id" gorm:"index;not null"`
	AgentUserID    int64      `json:"agent_user_id" gorm:"index;not null"`
	ConversationID int64      `json:"conversation_id" gorm:"index;not null"`
	TriggerUserID  int64      `json:"trigger_user_id" gorm:"index;not null"`
	SourceEventID  string     `json:"source_event_id" gorm:"size:100;uniqueIndex;not null"`
	TraceID        string     `json:"trace_id" gorm:"size:100;index"`
	EventType      string     `json:"event_type" gorm:"size:80;index"`
	Status         string     `json:"status" gorm:"size:30;index;not null;default:queued"`
	ErrorMessage   string     `json:"error_message" gorm:"type:text"`
	HeartbeatAt    *time.Time `json:"heartbeat_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (t *AgentTask) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&t.ID)
}

func (AgentTask) TableName() string {
	return "agent_tasks"
}

// BeforeCreate 在插入权限记录前补充分布式雪花 ID。
func (p *BotPermission) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&p.ID)
}

// TableName 固定历史权限表名 bot_permissions，兼容已有数据。
func (BotPermission) TableName() string {
	return "bot_permissions"
}

// BotRoute 保存 Agent 路由规则。
// 表名和字段仍保留 bot 前缀以兼容历史结构，业务上用于 Agent 事件调度。
type BotRoute struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	BotID        int64     `json:"bot_id" gorm:"index;not null"`
	RoutePattern string    `json:"route_pattern" gorm:"size:255;not null"`
	RouteType    string    `json:"route_type" gorm:"size:50;not null;default:keyword"`
	Priority     int64     `json:"priority" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// BeforeCreate 在插入路由规则前补充分布式雪花 ID。
func (r *BotRoute) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName 固定历史路由表名 bot_routes。
func (BotRoute) TableName() string {
	return "bot_routes"
}

// BillingRecord 保存一次 Agent 行为的实际 token 用量和费用。
type BillingRecord struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	BotID          int64     `json:"bot_id" gorm:"index;not null"`
	UserID         int64     `json:"user_id" gorm:"index;not null"`
	ConversationID int64     `json:"conversation_id" gorm:"index;default:0"`
	Action         string    `json:"action" gorm:"size:50;not null"`
	TokenCount     int64     `json:"token_count" gorm:"default:0"`
	InputTokens    int64     `json:"input_tokens" gorm:"default:0"`
	OutputTokens   int64     `json:"output_tokens" gorm:"default:0"`
	Cost           float64   `json:"cost" gorm:"default:0"`
	ModelName      string    `json:"model_name" gorm:"size:100"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// BeforeCreate 在插入计费记录前补充分布式雪花 ID。
func (r *BillingRecord) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName 固定计费记录表名。
func (BillingRecord) TableName() string {
	return "billing_records"
}

// fillSnowflakeID 在模型 ID 为空时统一补充分布式雪花 ID。
func fillSnowflakeID(id *int64) error {
	if *id != 0 {
		return nil
	}
	nextID, err := idgen.NextID()
	if err != nil {
		return err
	}
	*id = nextID
	return nil
}
