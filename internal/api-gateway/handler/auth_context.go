package handler

import (
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
)

// currentUserID 从 JWT 中间件写入的 Hertz 上下文读取登录用户 ID。
// 这里不解析 token，只信任已通过 middleware.JWTAuthMiddleware 校验后的 userID。
func currentUserID(c *app.RequestContext) (int64, bool) {
	value, ok := c.Get("userID")
	if !ok {
		return 0, false
	}
	id, ok := value.(int64)
	if !ok || id <= 0 {
		return 0, false
	}
	return id, true
}

// requireCurrentUserID 是 handler 的认证门闩。
// 读取失败时直接写 401 响应，并用 bool 告诉调用方停止后续业务处理。
func requireCurrentUserID(c *app.RequestContext) (int64, bool) {
	id, ok := currentUserID(c)
	if !ok {
		response.Unauthorized(c, "无效的用户ID")
	}
	return id, ok
}

// currentUsername 从 JWT 中间件写入的 Hertz 上下文读取登录用户名。
// 该值只作为展示和用户级文件夹命名辅助，权限判断仍以 userID 为准。
func currentUsername(c *app.RequestContext) string {
	value, ok := c.Get("username")
	if !ok {
		return ""
	}
	username, _ := value.(string)
	return username
}

// userInfoLookupOK 统一判断 user-service 查询是否拿到了可用用户资料。
// 网关批量补充昵称、头像时用它区分“未找到/下游失败”和正常响应。
func userInfoLookupOK(resp *user.GetUserInfoResp, err error) bool {
	return err == nil && resp != nil && resp.Success
}
