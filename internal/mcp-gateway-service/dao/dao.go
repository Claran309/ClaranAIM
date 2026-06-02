// Package dao 实现 mcp-gateway-service 的持久化访问。
package dao

import (
	"ClaranAIM/internal/mcp-gateway-service/model"
	"context"
	"errors"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 打开 MySQL 并迁移 MCP 工具调用审计表。
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.ToolCallTrace{}); err != nil {
		return nil, err
	}
	return db, nil
}

// TraceFilter 限定工具调用审计查询条件。
type TraceFilter struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
	Limit          int
	Offset         int
}

// Repository 定义 MCP Gateway 的持久化操作。
type Repository interface {
	SaveTrace(ctx context.Context, trace *model.ToolCallTrace) error
	GetTraceByTraceID(ctx context.Context, userID int64, traceID string) (*model.ToolCallTrace, error)
	ListTraces(ctx context.Context, filter TraceFilter) ([]model.ToolCallTrace, int64, error)
}

type repositoryImpl struct {
	db *gorm.DB
}

// NewRepository 创建 GORM 仓储。
func NewRepository(db *gorm.DB) Repository {
	return &repositoryImpl{db: db}
}

// SaveTrace 写入或覆盖一条工具调用审计。
func (r *repositoryImpl) SaveTrace(ctx context.Context, trace *model.ToolCallTrace) error {
	return r.db.WithContext(ctx).Save(trace).Error
}

// GetTraceByTraceID 按 trace_id 读取当前用户可见的审计记录。
func (r *repositoryImpl) GetTraceByTraceID(ctx context.Context, userID int64, traceID string) (*model.ToolCallTrace, error) {
	var trace model.ToolCallTrace
	err := r.db.WithContext(ctx).Where("user_id = ? AND trace_id = ?", userID, traceID).First(&trace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &trace, err
}

// ListTraces 查询工具调用记录。
// UserID > 0 时按当前用户裁剪；UserID == 0 是 admin-service 的全局审计查询约定，
// 普通用户入口会由 api-gateway 注入真实用户 ID。
func (r *repositoryImpl) ListTraces(ctx context.Context, filter TraceFilter) ([]model.ToolCallTrace, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.ToolCallTrace{})
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.AgentID > 0 {
		query = query.Where("agent_id = ?", filter.AgentID)
	}
	if filter.ConversationID > 0 {
		query = query.Where("conversation_id = ?", filter.ConversationID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var traces []model.ToolCallTrace
	err := query.Order("created_at DESC, id DESC").Limit(limit).Offset(filter.Offset).Find(&traces).Error
	return traces, total, err
}
