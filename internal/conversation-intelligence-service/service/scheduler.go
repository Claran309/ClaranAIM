package service

import (
	"context"
	"log"
	"time"
)

// StartDigestScheduler 周期性扫描活跃会话游标，把达到阈值的窗口归档成长期聊天 RAG。
func StartDigestScheduler(ctx context.Context, svc ConversationIntelligenceService, options DigestScheduleOptions, interval time.Duration) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := svc.RunDueDigestJobs(ctx, options); err != nil && ctx.Err() == nil {
					log.Printf("conversation-intelligence调度归档失败: %v", err)
				}
			}
		}
	}()
}
