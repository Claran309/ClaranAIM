// Package model 定义 Agent 个性化使用的持久化记忆事实。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// 记忆事实的分类枚举。
// Scope 控制隔离粒度，Visibility 控制读取权限，VectorStatus 预留给后续向量库异步处理流程。
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

// MemoryFact 保存一条从聊天、Agent 输出或用户输入中提取的可编辑事实。
type MemoryFact struct {
	ID             int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	BotID          int64          `json:"bot_id" gorm:"index:idx_memory_scope,priority:1;not null"`
	UserID         int64          `json:"user_id" gorm:"index:idx_memory_scope,priority:2;index;not null"`
	OwnerUserID    int64          `json:"owner_user_id" gorm:"index;not null"`
	GroupID        int64          `json:"group_id" gorm:"index:idx_memory_scope,priority:3;index"`
	ConversationID int64          `json:"conversation_id" gorm:"index:idx_memory_scope,priority:4;index"`
	SessionID      string         `json:"session_id" gorm:"size:160;index:idx_memory_scope,priority:5"`
	Scope          string         `json:"scope" gorm:"size:32;index;not null"`
	Type           string         `json:"type" gorm:"size:64;index;not null"`
	Title          string         `json:"title" gorm:"size:160"`
	Content        string         `json:"content" gorm:"type:text;not null"`
	Source         string         `json:"source" gorm:"size:255"`
	Visibility     string         `json:"visibility" gorm:"size:32;index;not null;default:private"`
	Enabled        bool           `json:"enabled" gorm:"not null;default:true;index"`
	VectorStatus   string         `json:"vector_status" gorm:"size:32;not null;default:pending"`
	EmbeddingRef   string         `json:"embedding_ref" gorm:"size:255"`
	Confidence     float64        `json:"confidence" gorm:"not null;default:0"`
	LastUsedAt     *time.Time     `json:"last_used_at"`
	CreatedAt      time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 固定表名，避免 GORM 根据结构体名推导成 memory_facts 以外的名称。
func (MemoryFact) TableName() string {
	return "memory_facts"
}

// BeforeCreate 为记忆事实补充分布式雪花 ID。
func (m *MemoryFact) BeforeCreate(tx *gorm.DB) error {
	if m.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	if err != nil {
		return err
	}
	m.ID = id
	return nil
}
