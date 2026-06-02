// Package model 定义 mcp-gateway-service 的工具调用审计模型。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

const (
	TraceStatusSuccess = "success"
	TraceStatusFailed  = "failed"
)

// ToolCallTrace 记录一次 MCP 工具调用的审计摘要。
type ToolCallTrace struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	UserID         int64     `json:"user_id" gorm:"index;not null"`
	AgentID        int64     `json:"agent_id" gorm:"index;not null;default:0"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null;default:0"`
	ToolName       string    `json:"tool_name" gorm:"size:160;index;not null"`
	Source         string    `json:"source" gorm:"size:32;index;not null"`
	ServerName     string    `json:"server_name" gorm:"size:160"`
	TraceID        string    `json:"trace_id" gorm:"size:128;uniqueIndex"`
	Status         string    `json:"status" gorm:"size:32;index;not null"`
	LatencyMS      int64     `json:"latency_ms" gorm:"not null;default:0"`
	ErrorMessage   string    `json:"error_message" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 固定 MCP trace 表名。
func (ToolCallTrace) TableName() string {
	return "mcp_tool_call_traces"
}

// BeforeCreate 使用项目统一雪花 ID。
func (t *ToolCallTrace) BeforeCreate(tx *gorm.DB) error {
	if t.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	t.ID = id
	return err
}
