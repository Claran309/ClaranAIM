package handler

import (
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
)

// currentUserID 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
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

// requireCurrentUserID 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func requireCurrentUserID(c *app.RequestContext) (int64, bool) {
	id, ok := currentUserID(c)
	if !ok {
		response.Unauthorized(c, "无效的用户ID")
	}
	return id, ok
}

// userInfoLookupOK 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func userInfoLookupOK(resp *user.GetUserInfoResp, err error) bool {
	return err == nil && resp != nil && resp.Success
}
