package handler

import (
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
)

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

func requireCurrentUserID(c *app.RequestContext) (int64, bool) {
	id, ok := currentUserID(c)
	if !ok {
		response.Unauthorized(c, "无效的用户ID")
	}
	return id, ok
}

func userInfoLookupOK(resp *user.GetUserInfoResp, err error) bool {
	return err == nil && resp != nil && resp.Success
}
