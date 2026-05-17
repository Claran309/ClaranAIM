// Package handler 实现 API 网关的 HTTP 请求处理层
// 每个方法对应一个路由端点，负责：解析请求参数 → 调用 RPC 客户端 → 返回统一响应
// 本层不包含业务逻辑，业务逻辑在各微服务的 Service 层中实现
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/response"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

// UserHandler 处理所有用户相关的 HTTP 请求
type UserHandler struct{}

// NewUserHandler 创建用户 HTTP handler。
//
// handler 不直接持有 DAO 或 Service，职责限定为请求参数解析、JWT 上下文读取、
// RPC 调用和统一响应封装，便于 api-gateway 与用户服务保持清晰边界。
func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

func bindUserJSONUseNumber(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

func parseOptionalJSONNumber(value json.Number, name string) (int64, error) {
	if value.String() == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil || id < 0 {
		return 0, errors.New("无效的" + name)
	}
	return id, nil
}

// Register 用户注册
// POST /api/v1/user/register
// 请求体: {username, password, nickname}
// 流程: 解析请求 → RPC 调用 user-service.Register → 返回注册结果
func (h *UserHandler) Register(ctx context.Context, c *app.RequestContext) {
	type registerReq struct {
		Username string `json:"username"` // 用户名，唯一标识
		Password string `json:"password"` // 密码，明文传输，服务端 bcrypt 加密
		Nickname string `json:"nickname"` // 昵称，为空时默认等于 username
	}
	var req registerReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	// 通过 RPC 调用 user-service 完成注册
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

// Login 用户登录
// POST /api/v1/user/login
// 请求体: {username, password}
// 流程: 解析请求 → RPC 调用 user-service.Login → 返回 JWT Token
func (h *UserHandler) Login(ctx context.Context, c *app.RequestContext) {
	type loginReq struct {
		Username string `json:"username"` // 用户名
		Password string `json:"password"` // 密码
	}
	var req loginReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	// 通过 RPC 调用 user-service 完成登录验证
	resp, err := client.UserClient.Login(ctx, client.NewLoginReq(req.Username, req.Password))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{
		"success":       resp.Success,
		"token":         resp.Token,
		"access_token":  firstNonEmpty(resp.AccessToken, resp.Token),
		"refresh_token": resp.RefreshToken,
		"user_id":       resp.UserId,
		"role":          resp.Role,
		"msg":           resp.Msg,
	})
}

// RefreshToken exchanges a refresh token for a new access token.
//
// The refresh token is parsed locally to verify its type and signature, then the
// user is fetched from user-service so disabled/deleted users cannot keep
// refreshing access tokens forever.
func (h *UserHandler) RefreshToken(ctx context.Context, c *app.RequestContext) {
	type refreshReq struct {
		RefreshToken string `json:"refresh_token"`
	}
	var req refreshReq
	if err := c.BindJSON(&req); err != nil || req.RefreshToken == "" {
		response.BadRequest(c, "refresh_token不能为空")
		return
	}
	claims, err := jwt.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		response.Unauthorized(c, "无效的RefreshToken")
		return
	}
	userResp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(claims.UserID))
	if err != nil || userResp == nil || !userResp.Success || userResp.User == nil {
		response.Unauthorized(c, "用户不存在或已被禁用")
		return
	}
	role := userResp.User.Role
	if role == "" {
		role = jwt.RoleUser
	}
	accessToken, err := jwt.GenerateAccessToken(jwt.GetSecretKey(), claims.UserID, claims.Username, role, jwt.GetAccessExpirationHours())
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{
		"success":      true,
		"token":        accessToken,
		"access_token": accessToken,
		"user_id":      claims.UserID,
		"role":         role,
	})
}

// GetUserInfo 获取当前登录用户的信息
// GET /api/v1/user/info
// 无请求参数，userID 从 JWT Token 中提取
// 流程: 从上下文获取 userID → RPC 调用 user-service.GetUserInfo → 返回用户信息
func (h *UserHandler) GetUserInfo(ctx context.Context, c *app.RequestContext) {
	// JWTAuthMiddleware 已将 userID 注入上下文
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// UpdateUserInfo 更新当前用户信息
// PUT /api/v1/user/info
// 请求体支持 nickname、email、phone、avatar、cover、signature、bio、location、website、gender、birthday。
// 前端个人资料页使用覆盖式保存，因此空字符串也会写入，用于清空资料字段。
func (h *UserHandler) UpdateUserInfo(ctx context.Context, c *app.RequestContext) {
	type updateReq struct {
		Nickname  string `json:"nickname"`  // 新昵称
		Email     string `json:"email"`     // 新邮箱
		Phone     string `json:"phone"`     // 新手机号
		Avatar    string `json:"avatar"`    // 头像URL
		Cover     string `json:"cover"`     // 个人主页头图URL
		Signature string `json:"signature"` // 个性签名
		Bio       string `json:"bio"`       // 个人简介
		Location  string `json:"location"`  // 所在地
		Website   string `json:"website"`   // 个人主页/网站
		Gender    string `json:"gender"`    // 性别/展示身份
		Birthday  string `json:"birthday"`  // 生日，YYYY-MM-DD
	}
	var req updateReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.UpdateUserInfo(ctx, client.NewUpdateUserInfoReq(
		id,
		req.Nickname,
		req.Email,
		req.Phone,
		req.Avatar,
		req.Cover,
		req.Signature,
		req.Bio,
		req.Location,
		req.Website,
		req.Gender,
		req.Birthday,
		true,
	))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// UpdateAvatar 更新用户头像
// POST /api/v1/user/avatar
// 请求体: {avatar}
// avatar 为头像URL字符串
func (h *UserHandler) UpdateAvatar(ctx context.Context, c *app.RequestContext) {
	type avatarReq struct {
		Avatar string `json:"avatar"` // 头像URL
	}
	var req avatarReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.UpdateAvatar(ctx, &user.UpdateAvatarReq{UserId: id, Avatar: req.Avatar})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// Logout 用户登出
// POST /api/v1/user/logout
// 将用户在线状态切换为 offline，清除 Redis 中的在线记录
func (h *UserHandler) Logout(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.UpdateStatus(ctx, &user.UpdateStatusReq{UserId: id, Status: "offline"})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// AddFriend 添加好友
// POST /api/v1/user/friend/add
// 请求体: {friend_id, group_id, remark}
// 好友关系是双向的：A 添加 B 时，B 的好友列表也会出现 A
func (h *UserHandler) AddFriend(ctx context.Context, c *app.RequestContext) {
	type addFriendReq struct {
		FriendID json.Number `json:"friend_id"` // 好友的用户ID
		GroupID  json.Number `json:"group_id"`  // 好友分组ID，0 表示默认分组
		Remark   string      `json:"remark"`    // 好友备注名
	}
	var req addFriendReq
	if err := bindUserJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	friendID, err := parseOptionalJSONNumber(req.FriendID, "好友用户ID")
	if err != nil || friendID <= 0 {
		response.BadRequest(c, "无效的好友用户ID")
		return
	}
	groupID, err := parseOptionalJSONNumber(req.GroupID, "好友分组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.AddFriend(ctx, client.NewAddFriendReq(id, friendID, groupID, req.Remark))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteFriend 删除好友
// POST /api/v1/user/friend/delete
// 请求体: {friend_id}
// 双向删除：A 删除 B 时，B 的好友列表也会移除 A
func (h *UserHandler) DeleteFriend(ctx context.Context, c *app.RequestContext) {
	type deleteFriendReq struct {
		FriendID json.Number `json:"friend_id"` // 要删除的好友用户ID
	}
	var req deleteFriendReq
	if err := bindUserJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	friendID, err := parseOptionalJSONNumber(req.FriendID, "好友用户ID")
	if err != nil || friendID <= 0 {
		response.BadRequest(c, "无效的好友用户ID")
		return
	}

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.DeleteFriend(ctx, client.NewDeleteFriendReq(id, friendID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// UpdateFriendRemark 修改好友备注和分组。
// PUT /api/v1/user/friend/remark
// 请求体: {friend_id, group_id, remark}
func (h *UserHandler) UpdateFriendRemark(ctx context.Context, c *app.RequestContext) {
	type updateFriendRemarkReq struct {
		FriendID json.Number `json:"friend_id"`
		GroupID  json.Number `json:"group_id"`
		Remark   string      `json:"remark"`
	}
	var req updateFriendRemarkReq
	if err := bindUserJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	friendID, err := parseOptionalJSONNumber(req.FriendID, "好友用户ID")
	if err != nil || friendID <= 0 {
		response.BadRequest(c, "friend_id不能为空")
		return
	}
	groupID, err := parseOptionalJSONNumber(req.GroupID, "好友分组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.UpdateFriendRemark(ctx, client.NewUpdateFriendRemarkReq(id, friendID, groupID, req.Remark))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetFriendList returns the current user's friends with remarks and status.
func (h *UserHandler) GetFriendList(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.GetFriendList(ctx, client.NewGetFriendListReq(id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// CreateFriendGroup 创建好友分组
// POST /api/v1/user/friend/group
// 请求体: {name}
// 好友分组用于对好友进行分类管理，如"同事""家人""同学"
func (h *UserHandler) CreateFriendGroup(ctx context.Context, c *app.RequestContext) {
	type createGroupReq struct {
		Name string `json:"name"` // 分组名称
	}
	var req createGroupReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.CreateFriendGroup(ctx, &user.CreateFriendGroupReq{UserId: id, Name: req.Name})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetFriendGroups 获取好友分组列表
// GET /api/v1/user/friend/groups
// 返回当前用户创建的所有好友分组
func (h *UserHandler) GetFriendGroups(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.UserClient.GetFriendGroups(ctx, &user.GetFriendGroupsReq{UserId: id})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// BatchGetUserInfo 批量获取用户信息
// GET /api/v1/user/batch?ids=1,2,3
// 根据用户ID列表批量获取用户信息，用于聊天中解析发送者昵称和头像
func (h *UserHandler) BatchGetUserInfo(ctx context.Context, c *app.RequestContext) {
	idsStr := c.Query("ids")
	if idsStr == "" {
		response.BadRequest(c, "ids参数不能为空")
		return
	}

	var ids []int64
	for _, s := range splitIDs(idsStr) {
		id, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		response.BadRequest(c, "无效的用户ID")
		return
	}

	resp, err := client.UserClient.BatchGetUserInfo(ctx, client.NewBatchGetUserInfoReq(ids))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// splitIDs 将逗号分隔的ID字符串拆分为字符串切片
func splitIDs(s string) []string {
	var result []string
	for _, part := range splitByComma(s) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitByComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
