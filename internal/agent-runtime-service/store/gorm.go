package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InitDB 打开 runtime 的 MySQL 连接并执行非破坏性迁移。
func InitDB(dsn string) (*gorm.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("agent-runtime-service mysql dsn 为空")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	models := []interface{}{
		&AgentSession{},
		&AgentMessage{},
		&AgentRun{},
		&AgentCheckpoint{},
	}
	for _, model := range models {
		if err := db.AutoMigrate(model); err != nil {
			return nil, err
		}
	}
	return db, nil
}

// GormStore 是 runtime 的 MySQL 持久化实现，同时提供会话、运行、审批和 checkpoint。
type GormStore struct {
	db *gorm.DB
}

// NewGormStore 创建基于 GORM 的 runtime 存储。
func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

// GetSession 读取会话和全部消息，主要用于诊断和测试。
func (s *GormStore) GetSession(ctx context.Context, id string) (*Session, error) {
	var session AgentSession
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var messages []AgentMessage
	if err := s.db.WithContext(ctx).Where("session_id = ?", id).Order("id ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	return sessionFromModel(session, messages), nil
}

// AppendMessage 追加一条消息，并在消息数超过阈值时生成滚动摘要。
func (s *GormStore) AppendMessage(ctx context.Context, id string, owner SessionOwner, role, content string, tokenCount int64) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("session id 不能为空")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		session, err := ensureSession(tx, id, owner)
		if err != nil {
			return err
		}
		message := AgentMessage{SessionID: id, Role: role, Content: content, TokenCount: tokenCount}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{
			"message_count": gorm.Expr("message_count + ?", 1),
			"updated_at":    time.Now(),
		}
		if strings.TrimSpace(session.Title) == "" && role == "user" {
			updates["title"] = makeSessionTitle(content)
		}
		if err := tx.Model(&AgentSession{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return maybeSummarize(tx, id, DefaultContextLimit, DefaultSummaryTrigger)
	})
}

// Context 返回滚动摘要和最近 limit 条未摘要消息。
func (s *GormStore) Context(ctx context.Context, id string, limit int) (string, []Message, error) {
	var session AgentSession
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	if limit <= 0 {
		limit = DefaultContextLimit
	}
	var messages []AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND id > ?", id, session.SummarizedThrough).
		Order("id ASC").
		Find(&messages).Error; err != nil {
		return "", nil, err
	}
	if len(messages) == 0 && strings.TrimSpace(session.Summary) == "" && session.SummarizedThrough == 0 {
		if err := s.db.WithContext(ctx).
			Where("session_id = ?", id).
			Order("id DESC").
			Limit(limit).
			Find(&messages).Error; err != nil {
			return "", nil, err
		}
		reverseMessages(messages)
	}
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return session.Summary, messagesFromModel(messages), nil
}

// ListSessions 返回会话元数据，过滤条件为 0 时表示不限制。
func (s *GormStore) ListSessions(ctx context.Context, owner SessionOwner) ([]SessionMeta, error) {
	query := s.db.WithContext(ctx).Model(&AgentSession{})
	if owner.BotID > 0 {
		query = query.Where("bot_id = ?", owner.BotID)
	}
	if owner.UserID > 0 {
		query = query.Where("user_id = ?", owner.UserID)
	}
	if owner.ConversationID > 0 {
		query = query.Where("conversation_id = ?", owner.ConversationID)
	}
	var sessions []AgentSession
	if err := query.Order("updated_at DESC").Limit(100).Find(&sessions).Error; err != nil {
		return nil, err
	}
	out := make([]SessionMeta, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, SessionMeta{
			ID:        session.ID,
			Title:     session.Title,
			Summary:   session.Summary,
			CreatedAt: session.CreatedAt,
		})
	}
	return out, nil
}

// DeleteSession 删除会话及其消息。
func (s *GormStore) DeleteSession(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&AgentMessage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&AgentSession{}).Error
	})
}

// CreateRun 创建一次运行记录。
func (s *GormStore) CreateRun(ctx context.Context, run *RunInfo) error {
	if run == nil || strings.TrimSpace(run.RunID) == "" {
		return errors.New("run_id 不能为空")
	}
	return s.db.WithContext(ctx).Create(runToModel(run)).Error
}

// UpdateRun 覆盖更新一次运行记录。
func (s *GormStore) UpdateRun(ctx context.Context, run *RunInfo) error {
	if run == nil || strings.TrimSpace(run.RunID) == "" {
		return errors.New("run_id 不能为空")
	}
	return s.db.WithContext(ctx).Save(runToModel(run)).Error
}

// GetRun 按 run_id 查询运行记录。
func (s *GormStore) GetRun(ctx context.Context, runID string) (*RunInfo, error) {
	var run AgentRun
	err := s.db.WithContext(ctx).Where("run_id = ?", runID).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	info := runFromModel(run)
	return &info, nil
}

// ListRuns 返回最近运行记录。
func (s *GormStore) ListRuns(ctx context.Context, filter RunFilter) ([]RunInfo, error) {
	query := s.db.WithContext(ctx).Model(&AgentRun{})
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.BotID > 0 {
		query = query.Where("bot_id = ?", filter.BotID)
	}
	if filter.ConversationID > 0 {
		query = query.Where("conversation_id = ?", filter.ConversationID)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var runs []AgentRun
	if err := query.Order("started_at DESC").Limit(limit).Find(&runs).Error; err != nil {
		return nil, err
	}
	out := make([]RunInfo, 0, len(runs))
	for _, run := range runs {
		out = append(out, runFromModel(run))
	}
	return out, nil
}

// Get checkpoint 读取 Eino checkpoint。
func (s *GormStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	var checkpoint AgentCheckpoint
	err := s.db.WithContext(ctx).Where("checkpoint_id = ?", checkPointID).First(&checkpoint).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return checkpoint.Data, true, nil
}

// Set checkpoint 写入 Eino checkpoint，支持跨 runtime 实例恢复。
func (s *GormStore) Set(ctx context.Context, checkPointID string, data []byte) error {
	if strings.TrimSpace(checkPointID) == "" {
		return errors.New("checkpoint id 不能为空")
	}
	return s.db.WithContext(ctx).Save(&AgentCheckpoint{CheckPointID: checkPointID, Data: data}).Error
}

func ensureSession(tx *gorm.DB, id string, owner SessionOwner) (*AgentSession, error) {
	var session AgentSession
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		session = AgentSession{
			ID:             id,
			BotID:          owner.BotID,
			UserID:         owner.UserID,
			ConversationID: owner.ConversationID,
		}
		if createErr := tx.Create(&session).Error; createErr != nil {
			return nil, createErr
		}
		return &session, nil
	}
	if err != nil {
		return nil, err
	}
	if owner.BotID > 0 && session.BotID != 0 && session.BotID != owner.BotID {
		return nil, fmt.Errorf("session %s 属于 bot %d，拒绝 bot %d 写入", id, session.BotID, owner.BotID)
	}
	if owner.UserID > 0 && session.UserID != 0 && session.UserID != owner.UserID {
		return nil, fmt.Errorf("session %s 属于 user %d，拒绝 user %d 写入", id, session.UserID, owner.UserID)
	}
	return &session, nil
}

func maybeSummarize(tx *gorm.DB, id string, keepRecent, trigger int) error {
	if keepRecent <= 0 {
		keepRecent = DefaultContextLimit
	}
	if trigger <= keepRecent {
		trigger = keepRecent * 2
	}
	var session AgentSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&session).Error; err != nil {
		return err
	}
	var count int64
	if err := tx.Model(&AgentMessage{}).Where("session_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count <= int64(trigger) {
		return nil
	}
	var cutoff AgentMessage
	err := tx.Where("session_id = ? AND id > ?", id, session.SummarizedThrough).
		Order("id DESC").
		Offset(keepRecent).
		Limit(1).
		Find(&cutoff).Error
	if err != nil {
		return err
	}
	if cutoff.ID == 0 {
		return nil
	}
	var messages []AgentMessage
	if err := tx.Where("session_id = ? AND id > ? AND id <= ?", id, session.SummarizedThrough, cutoff.ID).
		Order("id ASC").
		Find(&messages).Error; err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	summary := buildRollingSummary(session.Summary, messagesFromModel(messages), DefaultSummaryMaxRunes)
	return tx.Model(&AgentSession{}).Where("id = ?", id).Updates(map[string]interface{}{
		"summary":            summary,
		"summarized_through": cutoff.ID,
		"updated_at":         time.Now(),
	}).Error
}

func reverseMessages(messages []AgentMessage) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}

func sessionFromModel(session AgentSession, messages []AgentMessage) *Session {
	return &Session{
		ID:        session.ID,
		Title:     session.Title,
		Summary:   session.Summary,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
		Messages:  messagesFromModel(messages),
	}
}

func messagesFromModel(messages []AgentMessage) []Message {
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		out = append(out, Message{
			ID:         message.ID,
			Role:       message.Role,
			Content:    message.Content,
			TokenCount: message.TokenCount,
			CreatedAt:  message.CreatedAt,
		})
	}
	return out
}

func runToModel(run *RunInfo) *AgentRun {
	return &AgentRun{
		RunID:           run.RunID,
		SessionID:       run.SessionID,
		BotID:           run.BotID,
		UserID:          run.UserID,
		ConversationID:  run.ConversationID,
		Status:          run.Status,
		Step:            run.Step,
		InputTokens:     run.InputTokens,
		OutputTokens:    run.OutputTokens,
		CostUSD:         run.CostUSD,
		InterruptReason: run.InterruptReason,
		ApprovalID:      run.ApprovalID,
		InterruptedTool: run.InterruptedTool,
		CheckpointJSON:  run.CheckpointJSON,
		ReflectionJSON:  run.ReflectionJSON,
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
	}
}

func runFromModel(run AgentRun) RunInfo {
	return RunInfo{
		RunID:           run.RunID,
		SessionID:       run.SessionID,
		BotID:           run.BotID,
		UserID:          run.UserID,
		ConversationID:  run.ConversationID,
		Status:          run.Status,
		Step:            run.Step,
		InputTokens:     run.InputTokens,
		OutputTokens:    run.OutputTokens,
		CostUSD:         run.CostUSD,
		InterruptReason: run.InterruptReason,
		ApprovalID:      run.ApprovalID,
		InterruptedTool: run.InterruptedTool,
		CheckpointJSON:  run.CheckpointJSON,
		ReflectionJSON:  run.ReflectionJSON,
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
	}
}
