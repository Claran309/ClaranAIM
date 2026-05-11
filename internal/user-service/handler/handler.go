package handler

import (
	"ClaranAIM/internal/user-service/service"
	"ClaranAIM/kitex_gen/user"
	"context"
)

var (
	jwtSecret     string
	jwtExpiration int64
)

func InitJWTConfig(secret string, expiration int64) {
	jwtSecret = secret
	jwtExpiration = expiration
}

type UserServiceImpl struct {
	svc service.UserService
}

func NewUserServiceImpl(svc service.UserService) user.UserService {
	return &UserServiceImpl{svc: svc}
}

func (h *UserServiceImpl) Register(ctx context.Context, req *user.RegisterReq) (resp *user.RegisterResp, err error) {
	u, err := h.svc.Register(ctx, req.Username, req.Password, req.Nickname)
	if err != nil {
		return &user.RegisterResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.RegisterResp{Success: true, UserId: u.ID, Msg: "注册成功"}, nil
}

func (h *UserServiceImpl) Login(ctx context.Context, req *user.LoginReq) (resp *user.LoginResp, err error) {
	token, u, err := h.svc.Login(ctx, req.Username, req.Password, jwtSecret, jwtExpiration)
	if err != nil {
		return &user.LoginResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.LoginResp{Success: true, Token: token, UserId: u.ID, Msg: "登录成功"}, nil
}

func (h *UserServiceImpl) GetUserInfo(ctx context.Context, req *user.GetUserInfoReq) (resp *user.GetUserInfoResp, err error) {
	u, err := h.svc.GetUserInfo(ctx, req.UserId)
	if err != nil {
		return &user.GetUserInfoResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.GetUserInfoResp{
		Success: true,
		User: &user.User{
			Id:        u.ID,
			Username:  u.Username,
			Nickname:  u.Nickname,
			Avatar:    u.Avatar,
			Email:     u.Email,
			Phone:     u.Phone,
			Status:    u.Status,
			CreatedAt: u.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: u.UpdatedAt.Format("2006-01-02 15:04:05"),
		},
	}, nil
}

func (h *UserServiceImpl) UpdateUserInfo(ctx context.Context, req *user.UpdateUserInfoReq) (resp *user.UpdateUserInfoResp, err error) {
	err = h.svc.UpdateUserInfo(ctx, req.UserId, req.Nickname, req.Email, req.Phone)
	if err != nil {
		return &user.UpdateUserInfoResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.UpdateUserInfoResp{Success: true, Msg: "更新成功"}, nil
}

func (h *UserServiceImpl) UpdateAvatar(ctx context.Context, req *user.UpdateAvatarReq) (resp *user.UpdateAvatarResp, err error) {
	err = h.svc.UpdateAvatar(ctx, req.UserId, req.Avatar)
	if err != nil {
		return &user.UpdateAvatarResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.UpdateAvatarResp{Success: true, Msg: "头像更新成功"}, nil
}

func (h *UserServiceImpl) UpdateStatus(ctx context.Context, req *user.UpdateStatusReq) (resp *user.UpdateStatusResp, err error) {
	err = h.svc.UpdateStatus(ctx, req.UserId, req.Status)
	if err != nil {
		return &user.UpdateStatusResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.UpdateStatusResp{Success: true, Msg: "状态更新成功"}, nil
}

func (h *UserServiceImpl) AddFriend(ctx context.Context, req *user.AddFriendReq) (resp *user.AddFriendResp, err error) {
	err = h.svc.AddFriend(ctx, req.UserId, req.FriendId, req.GroupId, req.Remark)
	if err != nil {
		return &user.AddFriendResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.AddFriendResp{Success: true, Msg: "添加好友成功"}, nil
}

func (h *UserServiceImpl) DeleteFriend(ctx context.Context, req *user.DeleteFriendReq) (resp *user.DeleteFriendResp, err error) {
	err = h.svc.DeleteFriend(ctx, req.UserId, req.FriendId)
	if err != nil {
		return &user.DeleteFriendResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.DeleteFriendResp{Success: true, Msg: "删除好友成功"}, nil
}

func (h *UserServiceImpl) GetFriendList(ctx context.Context, req *user.GetFriendListReq) (resp *user.GetFriendListResp, err error) {
	friends, err := h.svc.GetFriendList(ctx, req.UserId)
	if err != nil {
		return &user.GetFriendListResp{Success: false, Msg: err.Error()}, nil
	}

	var friendList []*user.Friend
	for _, f := range friends {
		friendList = append(friendList, &user.Friend{
			Id:           f.ID,
			UserId:       f.UserID,
			FriendId:     f.FriendID,
			GroupId:      f.GroupID,
			Remark:       f.Remark,
			FriendName:   f.FriendName,
			FriendAvatar: f.FriendAvatar,
			FriendStatus: f.FriendStatus,
			GroupName:    f.GroupName,
		})
	}
	return &user.GetFriendListResp{Success: true, Friends: friendList}, nil
}

func (h *UserServiceImpl) UpdateFriendRemark(ctx context.Context, req *user.UpdateFriendRemarkReq) (resp *user.UpdateFriendRemarkResp, err error) {
	err = h.svc.UpdateFriendRemark(ctx, req.UserId, req.FriendId, 0, req.Remark)
	if err != nil {
		return &user.UpdateFriendRemarkResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.UpdateFriendRemarkResp{Success: true, Msg: "备注更新成功"}, nil
}

func (h *UserServiceImpl) CreateFriendGroup(ctx context.Context, req *user.CreateFriendGroupReq) (resp *user.CreateFriendGroupResp, err error) {
	g, err := h.svc.CreateFriendGroup(ctx, req.UserId, req.Name)
	if err != nil {
		return &user.CreateFriendGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.CreateFriendGroupResp{Success: true, GroupId: g.ID, Msg: "分组创建成功"}, nil
}

func (h *UserServiceImpl) MoveFriendGroup(ctx context.Context, req *user.MoveFriendGroupReq) (resp *user.MoveFriendGroupResp, err error) {
	err = h.svc.MoveFriendGroup(ctx, req.UserId, req.FriendId, req.GroupId)
	if err != nil {
		return &user.MoveFriendGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &user.MoveFriendGroupResp{Success: true, Msg: "移动分组成功"}, nil
}

func (h *UserServiceImpl) GetFriendGroups(ctx context.Context, req *user.GetFriendGroupsReq) (resp *user.GetFriendGroupsResp, err error) {
	groups, err := h.svc.GetFriendGroups(ctx, req.UserId)
	if err != nil {
		return &user.GetFriendGroupsResp{Success: false, Msg: err.Error()}, nil
	}

	var groupList []*user.FriendGroup
	for _, g := range groups {
		groupList = append(groupList, &user.FriendGroup{
			Id:     g.ID,
			UserId: g.UserID,
			Name:   g.Name,
		})
	}
	return &user.GetFriendGroupsResp{Success: true, Groups: groupList}, nil
}

func (h *UserServiceImpl) BatchGetUserInfo(ctx context.Context, req *user.BatchGetUserInfoReq) (resp *user.BatchGetUserInfoResp, err error) {
	users, err := h.svc.BatchGetUserInfo(ctx, req.UserIds)
	if err != nil {
		return &user.BatchGetUserInfoResp{Success: false, Msg: err.Error()}, nil
	}

	var userList []*user.User
	for _, u := range users {
		userList = append(userList, &user.User{
			Id:        u.ID,
			Username:  u.Username,
			Nickname:  u.Nickname,
			Avatar:    u.Avatar,
			Email:     u.Email,
			Phone:     u.Phone,
			Status:    u.Status,
			CreatedAt: u.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: u.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &user.BatchGetUserInfoResp{Success: true, Users: userList}, nil
}
