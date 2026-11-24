package service

import "context"

type sourceMessageIDKey struct{}

// WithSourceMessageID 把触发本轮 Agent 的 IM 消息 ID 传入运行链路，供记忆候选追溯来源。
func WithSourceMessageID(ctx context.Context, messageID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sourceMessageIDKey{}, messageID)
}

func sourceMessageIDs(ctx context.Context) []int64 {
	if ctx == nil {
		return nil
	}
	messageID, _ := ctx.Value(sourceMessageIDKey{}).(int64)
	if messageID <= 0 {
		return nil
	}
	return []int64{messageID}
}
