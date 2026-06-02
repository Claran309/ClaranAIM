// Package model 定义会话智能归档的持久化模型。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

const (
	JobStatusPending    = "pending"
	JobStatusProcessing = "processing"
	JobStatusCompleted  = "completed"
	JobStatusSkipped    = "skipped"
	JobStatusFailed     = "failed"

	ArtifactSummary         = "conversation_summary"
	ArtifactDecision        = "decision"
	ArtifactTask            = "task"
	ArtifactTopic           = "topic"
	ArtifactQuote           = "quote"
	ArtifactMemoryCandidate = "memory_candidate"
)

// DigestJob 表示一个会话窗口归档任务。
type DigestJob struct {
	ID             int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	ConversationID int64          `json:"conversation_id" gorm:"index;not null"`
	ViewerID       int64          `json:"viewer_id" gorm:"index;not null"`
	AgentID        int64          `json:"agent_id" gorm:"index"`
	StartMessageID int64          `json:"start_message_id" gorm:"index"`
	EndMessageID   int64          `json:"end_message_id" gorm:"index"`
	StartTime      *time.Time     `json:"start_time" gorm:"index"`
	EndTime        *time.Time     `json:"end_time" gorm:"index"`
	Reason         string         `json:"reason" gorm:"size:120"`
	Status         string         `json:"status" gorm:"size:32;index;not null;default:pending"`
	MessageCount   int            `json:"message_count"`
	ValuableCount  int            `json:"valuable_count"`
	ErrorMessage   string         `json:"error_message" gorm:"type:text"`
	RetryCount     int            `json:"retry_count" gorm:"not null;default:0"`
	MaxRetries     int            `json:"max_retries" gorm:"not null;default:3"`
	NextRunAt      *time.Time     `json:"next_run_at" gorm:"index"`
	LastAttemptAt  *time.Time     `json:"last_attempt_at" gorm:"index"`
	CompletedAt    *time.Time     `json:"completed_at" gorm:"index"`
	CreatedAt      time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

func (DigestJob) TableName() string {
	return "conversation_digest_jobs"
}

func (j *DigestJob) BeforeCreate(tx *gorm.DB) error {
	if j.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	if err != nil {
		return err
	}
	j.ID = id
	return nil
}

// ConversationArtifact 保存摘要、决策、任务、主题、金句和候选记忆等提炼产物。
type ConversationArtifact struct {
	ID             int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	JobID          int64          `json:"job_id" gorm:"index;not null"`
	ConversationID int64          `json:"conversation_id" gorm:"index;not null"`
	ViewerID       int64          `json:"viewer_id" gorm:"index;not null"`
	AgentID        int64          `json:"agent_id" gorm:"index"`
	ArtifactType   string         `json:"artifact_type" gorm:"size:64;index;not null"`
	Title          string         `json:"title" gorm:"size:180"`
	Content        string         `json:"content" gorm:"type:text;not null"`
	MetadataJSON   string         `json:"metadata_json" gorm:"type:text"`
	SourceMsgIDs   string         `json:"source_msg_ids" gorm:"type:text"`
	Confidence     float64        `json:"confidence" gorm:"not null;default:0.7"`
	RAGDocumentID  int64          `json:"rag_document_id" gorm:"index"`
	MemoryCandID   int64          `json:"memory_candidate_id" gorm:"index"`
	CreatedAt      time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

func (ConversationArtifact) TableName() string {
	return "conversation_artifacts"
}

func (a *ConversationArtifact) BeforeCreate(tx *gorm.DB) error {
	if a.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	if err != nil {
		return err
	}
	a.ID = id
	return nil
}

// ConversationActivityCursor 记录某个用户视角下的活跃会话归档游标。
// 它用于调度器判断是否达到“累计 N 条消息”或“窗口超过 M 分钟”的自动归档条件。
type ConversationActivityCursor struct {
	ID              int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	ConversationID  int64          `json:"conversation_id" gorm:"uniqueIndex:uniq_conversation_activity_scope,priority:1;not null"`
	ViewerID        int64          `json:"viewer_id" gorm:"uniqueIndex:uniq_conversation_activity_scope,priority:2;not null"`
	AgentID         int64          `json:"agent_id" gorm:"index"`
	FirstMessageID  int64          `json:"first_message_id" gorm:"index"`
	LastMessageID   int64          `json:"last_message_id" gorm:"index"`
	PendingMessages int            `json:"pending_messages" gorm:"not null;default:0"`
	WindowStartedAt time.Time      `json:"window_started_at" gorm:"index"`
	LastActivityAt  time.Time      `json:"last_activity_at" gorm:"index"`
	LastDigestJobID int64          `json:"last_digest_job_id" gorm:"index"`
	LastDigestAt    *time.Time     `json:"last_digest_at" gorm:"index"`
	CreatedAt       time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

func (ConversationActivityCursor) TableName() string {
	return "conversation_activity_cursors"
}

func (c *ConversationActivityCursor) BeforeCreate(tx *gorm.DB) error {
	if c.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}
