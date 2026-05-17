// Package middleware 实现 API 网关的 HTTP 中间件
// 提供 JWT 认证和 CORS 跨域支持
package middleware

import (
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/response"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

// JWTAuthMiddleware JWT 认证中间件
// 从请求头 Authorization 中提取 Bearer Token，验证签名和有效期
// 验证通过后将 userID 和 username 注入请求上下文，供后续 handler 使用
// 验证失败返回 401 状态码并中断请求
func JWTAuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// 提取 Authorization 头
		authHeader := string(c.GetHeader("Authorization"))
		if authHeader == "" {
			response.Unauthorized(c, "缺少认证信息")
			c.Abort()
			return
		}

		// 解析 "Bearer <token>" 格式
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c, "认证格式错误")
			c.Abort()
			return
		}

		// 解析并验证 JWT Token
		claims, err := jwt.ParseToken(parts[1])
		if err != nil {
			response.Unauthorized(c, "无效的Token")
			c.Abort()
			return
		}

		// 将用户信息注入上下文，后续 handler 通过 c.Get("userID") 获取
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next(ctx)
	}
}

// RequireRole 用于管理层接口的角色鉴权。当前项目还没有独立 admin 路由，
// 但预留这个中间件后，未来只需对 /admin 分组挂载 RequireRole(jwt.RoleAdmin)。
func RequireRole(roles ...string) app.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(ctx context.Context, c *app.RequestContext) {
		value, ok := c.Get("role")
		if !ok {
			response.Forbidden(c, "权限不足")
			c.Abort()
			return
		}
		role, ok := value.(string)
		if !ok {
			response.Forbidden(c, "权限不足")
			c.Abort()
			return
		}
		if _, ok := allowed[role]; !ok {
			response.Forbidden(c, "权限不足")
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

// CORSMiddleware 跨域资源共享中间件
// 开发阶段允许所有来源（*）的跨域请求
// 处理浏览器的 OPTIONS 预检请求，直接返回 204
func CORSMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Header("Access-Control-Allow-Origin", "*")                            // 允许所有来源
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS") // 允许的 HTTP 方法
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")  // 允许的请求头
		c.Header("Access-Control-Max-Age", "86400")                             // 预检请求缓存时间（24小时）

		// 浏览器预检请求（OPTIONS）直接返回 204，不进入业务逻辑
		if string(c.Method()) == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next(ctx)
	}
}

// RateLimitMiddleware 返回 API 网关级别的令牌桶限流中间件。
//
// 限流 key 优先使用 JWT 中间件写入的 userID，没有登录态时退化到客户端 IP。
// 当前实现是单进程内存令牌桶，适合开发阶段和单实例部署；多实例部署时应替换为
// Redis/Lua 或网关层限流，否则每个实例都会有独立配额。
func RateLimitMiddleware(enabled bool, capacity int, refillInterval time.Duration) app.HandlerFunc {
	limiter := newTokenBucketLimiter(capacity, refillInterval)
	return func(ctx context.Context, c *app.RequestContext) {
		if !enabled || limiter.allow(rateLimitKey(ctx, c)) {
			c.Next(ctx)
			return
		}
		c.AbortWithStatusJSON(429, map[string]interface{}{
			"code":    429,
			"message": "请求过于频繁，请稍后再试",
		})
	}
}

func rateLimitKey(ctx context.Context, c *app.RequestContext) string {
	if value, ok := c.Get("userID"); ok {
		switch id := value.(type) {
		case int64:
			return fmt.Sprintf("user:%d", id)
		case int:
			return fmt.Sprintf("user:%d", id)
		case string:
			if id != "" {
				return "user:" + id
			}
		}
	}
	ip := c.ClientIP()
	if ip == "" {
		ip = "unknown"
	}
	return "ip:" + ip
}

type tokenBucketLimiter struct {
	mu             sync.Mutex
	capacity       int
	refillInterval time.Duration
	buckets        map[string]*tokenBucket
}

type tokenBucket struct {
	tokens     int
	lastRefill time.Time
}

func newTokenBucketLimiter(capacity int, refillInterval time.Duration) *tokenBucketLimiter {
	if capacity <= 0 {
		capacity = 120
	}
	if refillInterval <= 0 {
		refillInterval = time.Minute
	}
	return &tokenBucketLimiter{
		capacity:       capacity,
		refillInterval: refillInterval,
		buckets:        make(map[string]*tokenBucket),
	}
}

func (l *tokenBucketLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	bucket := l.buckets[key]
	if bucket == nil {
		bucket = &tokenBucket{tokens: l.capacity, lastRefill: now}
		l.buckets[key] = bucket
	}
	if now.Sub(bucket.lastRefill) >= l.refillInterval {
		bucket.tokens = l.capacity
		bucket.lastRefill = now
	}
	if bucket.tokens <= 0 {
		return false
	}
	bucket.tokens--
	return true
}
