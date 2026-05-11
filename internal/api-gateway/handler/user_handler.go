package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/pkg/response"
	"context"

	"github.com/cloudwego/hertz/pkg/app"
)

type UserHandler struct{}

func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

func (h *UserHandler) Register(ctx context.Context, c *app.RequestContext) {
	type registerReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Nickname string `json:"nickname"`
	}
	var req registerReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	resp, err := client.UserClient.Register(ctx, client.NewRegisterReq(req.Username, req.Password, req.Nickname))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{
		"success": resp.Success,
		"user_id": resp.UserId,
		"msg":     resp.Msg,
	})
}

func (h *UserHandler) Login(ctx context.Context, c *app.RequestContext) {
	type loginReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var req loginReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	resp, err := client.UserClient.Login(ctx, client.NewLoginReq(req.Username, req.Password))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{
		"success": resp.Success,
		"token":   resp.Token,
		"user_id": resp.UserId,
		"msg":     resp.Msg,
	})
}

func (h *UserHandler) GetUserInfo(ctx context.Context, c *app.RequestContext) {
	userID, _ := c.Get("userID")
	id, ok := userID.(int64)
	if !ok {
		response.Unauthorized(c, "无效的用户ID")
		return
	}

	resp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *UserHandler) UpdateUserInfo(ctx context.Context, c *app.RequestContext) {
	type updateReq struct {
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
	}
	var req updateReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	userID, _ := c.Get("userID")
	id := userID.(int64)

	resp, err := client.UserClient.UpdateUserInfo(ctx, client.NewUpdateUserInfoReq(id, req.Nickname, req.Email, req.Phone))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *UserHandler) AddFriend(ctx context.Context, c *app.RequestContext) {
	type addFriendReq struct {
		FriendID int64  `json:"friend_id"`
		GroupID  int64  `json:"group_id"`
		Remark   string `json:"remark"`
	}
	var req addFriendReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	userID, _ := c.Get("userID")
	id := userID.(int64)

	resp, err := client.UserClient.AddFriend(ctx, client.NewAddFriendReq(id, req.FriendID, req.GroupID, req.Remark))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *UserHandler) DeleteFriend(ctx context.Context, c *app.RequestContext) {
	type deleteFriendReq struct {
		FriendID int64 `json:"friend_id"`
	}
	var req deleteFriendReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	userID, _ := c.Get("userID")
	id := userID.(int64)

	resp, err := client.UserClient.DeleteFriend(ctx, client.NewDeleteFriendReq(id, req.FriendID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *UserHandler) GetFriendList(ctx context.Context, c *app.RequestContext) {
	userID, _ := c.Get("userID")
	id := userID.(int64)

	resp, err := client.UserClient.GetFriendList(ctx, client.NewGetFriendListReq(id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *UserHandler) CreateFriendGroup(ctx context.Context, c *app.RequestContext) {
	type createGroupReq struct {
		Name string `json:"name"`
	}
	var req createGroupReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	userID, _ := c.Get("userID")
	id := userID.(int64)

	resp, err := client.UserClient.CreateFriendGroup(ctx, &user.CreateFriendGroupReq{UserId: id, Name: req.Name})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *UserHandler) GetFriendGroups(ctx context.Context, c *app.RequestContext) {
	userID, _ := c.Get("userID")
	id := userID.(int64)

	resp, err := client.UserClient.GetFriendGroups(ctx, &user.GetFriendGroupsReq{UserId: id})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
