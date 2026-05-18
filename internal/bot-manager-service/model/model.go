// Package model defines bot-manager-service persistence models.
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// Bot stores configuration needed to construct and run one agent instance.
type Bot struct {
	ID            int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Name          string    `json:"name" gorm:"size:100;not null"`
	Type          string    `json:"type" gorm:"size:20;not null;default:internal"`
	Description   string    `json:"description" gorm:"type:text"`
	ModelName     string    `json:"model_name" gorm:"size:100;not null"`
	APIKey        string    `json:"api_key" gorm:"size:255"`
	BaseURL       string    `json:"base_url" gorm:"size:255"`
	SystemPrompt  string    `json:"system_prompt" gorm:"type:text"`
	SkillsDir     string    `json:"skills_dir" gorm:"size:255"`
	AgentRoot     string    `json:"agent_root" gorm:"size:255"`
	AgentUserID   int64     `json:"agent_user_id" gorm:"index;default:0"`
	Avatar        string    `json:"avatar" gorm:"size:255"`
	Signature     string    `json:"signature" gorm:"size:120"`
	WorkspaceRoot string    `json:"workspace_root" gorm:"size:255"`
	ToolPolicy    string    `json:"tool_policy" gorm:"size:50;default:safe"`
	OwnerID       int64     `json:"owner_id" gorm:"index;not null"`
	IsActive      bool      `json:"is_active" gorm:"default:true"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate assigns a snowflake ID before inserting a bot.
func (b *Bot) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&b.ID)
}

// TableName keeps the bot table name explicit.
func (Bot) TableName() string {
	return "bots"
}

// BotPermission grants non-owner users controlled access to an Agent.
type BotPermission struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	BotID     int64     `json:"bot_id" gorm:"uniqueIndex:idx_bot_permission;not null"`
	UserID    int64     `json:"user_id" gorm:"uniqueIndex:idx_bot_permission;index;not null"`
	Role      string    `json:"role" gorm:"size:20;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// AgentDispatchRecord records one Kafka-triggered Agent invocation.
//
// The unique event_id + agent_user_id key makes the @Agent consumer idempotent:
// Kafka can redeliver a message after a crash, but an already completed Agent
// dispatch will not run the runtime again. The Agent reply also uses the same
// key as msg-core-service client_msg_id, covering the narrower crash window
// after reply persistence but before this record reaches "completed".
type AgentDispatchRecord struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	EventID        string    `json:"event_id" gorm:"size:64;uniqueIndex:uk_agent_dispatch_event_agent;not null"`
	AgentUserID    int64     `json:"agent_user_id" gorm:"uniqueIndex:uk_agent_dispatch_event_agent;index;not null"`
	BotID          int64     `json:"bot_id" gorm:"index;not null"`
	SourceMsgID    int64     `json:"source_msg_id" gorm:"index;not null"`
	ReplyMsgID     int64     `json:"reply_msg_id" gorm:"default:0"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`
	SenderID       int64     `json:"sender_id" gorm:"index;not null"`
	Status         string    `json:"status" gorm:"size:20;not null;default:started"`
	ErrorMessage   string    `json:"error_message" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate assigns a snowflake ID before inserting a dispatch record.
func (r *AgentDispatchRecord) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName keeps the dispatch table name explicit.
func (AgentDispatchRecord) TableName() string {
	return "agent_dispatch_records"
}

// BeforeCreate assigns a snowflake ID before inserting a permission.
func (p *BotPermission) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&p.ID)
}

// TableName keeps the permission table name explicit.
func (BotPermission) TableName() string {
	return "bot_permissions"
}

// BotRoute stores a future dispatch/routing rule for a bot.
type BotRoute struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	BotID        int64     `json:"bot_id" gorm:"index;not null"`
	RoutePattern string    `json:"route_pattern" gorm:"size:255;not null"`
	RouteType    string    `json:"route_type" gorm:"size:50;not null;default:keyword"`
	Priority     int64     `json:"priority" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// BeforeCreate assigns a snowflake ID before inserting a route.
func (r *BotRoute) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName keeps the route table name explicit.
func (BotRoute) TableName() string {
	return "bot_routes"
}

// BillingRecord stores one measured bot action cost.
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

// BeforeCreate assigns a snowflake ID before inserting a billing record.
func (r *BillingRecord) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName keeps the billing table name explicit.
func (BillingRecord) TableName() string {
	return "billing_records"
}

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
