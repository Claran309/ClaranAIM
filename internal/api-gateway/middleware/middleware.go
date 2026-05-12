// Package middleware 实现 API 网关的 HTTP 中间件
// 提供 JWT 认证和 CORS 跨域支持
package middleware

import (
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/response"
	"context"
	"strings"

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
		c.Next(ctx)
	}
}

// CORSMiddleware 跨域资源共享中间件
// 开发阶段允许所有来源（*）的跨域请求
// 处理浏览器的 OPTIONS 预检请求，直接返回 204
func CORSMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Header("Access-Control-Allow-Origin", "*")                                      // 允许所有来源
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")            // 允许的 HTTP 方法
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")             // 允许的请求头
		c.Header("Access-Control-Max-Age", "86400")                                        // 预检请求缓存时间（24小时）

		// 浏览器预检请求（OPTIONS）直接返回 204，不进入业务逻辑
		if string(c.Method()) == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next(ctx)
	}
}
