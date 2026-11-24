package store

import (
	"ClaranAIM/pkg/agentkit/component"
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// JSONLStore 是 MySQL 不可用时的本地回退实现，保留原有 JSONL 会话文件行为。
type JSONLStore struct {
	store *component.Store
}

// NewJSONLStore 创建 JSONL 会话存储。
func NewJSONLStore(dir string) (*JSONLStore, error) {
	store, err := component.NewStore(dir)
	if err != nil {
		return nil, err
	}
	return &JSONLStore{store: store}, nil
}

// GetSession 读取 JSONL 会话。
func (s *JSONLStore) GetSession(ctx context.Context, id string) (*Session, error) {
	session, err := s.store.GetSession(id)
	if err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(session.GetMessages()))
	for _, msg := range session.GetMessages() {
		if msg == nil {
			continue
		}
		messages = append(messages, Message{Role: string(msg.Role), Content: msg.Content})
	}
	return &Session{
		ID:        session.ID,
		Title:     session.GetTitle(),
		CreatedAt: session.CreatedAt,
		Messages:  messages,
	}, nil
}

// AppendMessage 追加 JSONL 消息。
func (s *JSONLStore) AppendMessage(ctx context.Context, id string, owner SessionOwner, role, content string, tokenCount int64) error {
	session, err := s.store.GetSession(id)
	if err != nil {
		return err
	}
	return session.Append(&schema.Message{Role: schema.RoleType(role), Content: content})
}

// Context 返回 JSONL 最近消息，历史摘要为空。
func (s *JSONLStore) Context(ctx context.Context, id string, limit int) (string, []Message, error) {
	session, err := s.store.GetSession(id)
	if err != nil {
		return "", nil, fmt.Errorf("读取 JSONL 会话失败: %w", err)
	}
	all := session.GetMessages()
	if limit <= 0 {
		limit = DefaultContextLimit
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	out := make([]Message, 0, len(all))
	for _, msg := range all {
		if msg == nil {
			continue
		}
		out = append(out, Message{Role: string(msg.Role), Content: msg.Content})
	}
	return "", out, nil
}

// ListSessions 返回按 Agent/用户/会话前缀过滤的 JSONL 会话元数据。
func (s *JSONLStore) ListSessions(ctx context.Context, owner SessionOwner) ([]SessionMeta, error) {
	all, err := s.store.ListSessions()
	if err != nil {
		return nil, err
	}
	prefix := ""
	if owner.BotID > 0 || owner.UserID > 0 || owner.ConversationID > 0 {
		prefix = fmt.Sprintf("agent_%d_user_%d_conv_%d", owner.BotID, owner.UserID, owner.ConversationID)
	}
	out := make([]SessionMeta, 0, len(all))
	for _, meta := range all {
		if prefix != "" && !strings.HasPrefix(meta.ID, prefix) {
			continue
		}
		out = append(out, SessionMeta{ID: meta.ID, Title: meta.Title, CreatedAt: meta.CreatedAt})
	}
	return out, nil
}

// DeleteSession 删除 JSONL 会话文件。
func (s *JSONLStore) DeleteSession(ctx context.Context, id string) error {
	return s.store.DeleteSession(id)
}
