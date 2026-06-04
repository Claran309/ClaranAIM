package middleware

import (
	"context"
	"net/http"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"go.uber.org/zap"
)

type Security struct {
	maxRequestsEveryMinute int
}

func NewSecurity(maxRequestsEveryMinute int) *Security {
	return &Security{maxRequestsEveryMinute: maxRequestsEveryMinute}
}

// UserRateLimitMiddleware 用户级限流
func (s *Security) UserRateLimitMiddleware() app.HandlerFunc {
	var (
		buckets = make(map[string]*TokenBucket)
		mu      sync.Mutex
	)

	return func(c context.Context, ctx *app.RequestContext) {
		// 获取用户IP
		userIP := ctx.ClientIP()

		// 令牌桶
		mu.Lock()
		bucket, ok := buckets[userIP]
		// 如果没有令牌桶，创建令牌桶
		if !ok {
			bucket = NewTokenBucket(50, 1)
			buckets[userIP] = bucket
		}
		mu.Unlock()

		// 令牌检查
		if !bucket.Allow() {
			zap.S().Errorf("请求过于频繁，请稍候再试")
			ctx.JSON(http.StatusTooManyRequests, utils.H{
				"msg": "请求过于频繁，请稍候再试",
			})
			ctx.Abort()
			return
		}

		ctx.Next(c)
	}
}
