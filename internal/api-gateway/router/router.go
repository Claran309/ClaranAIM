package router

import (
	"ClaranAIM/internal/api-gateway/handler"
	"ClaranAIM/internal/api-gateway/middleware"
	"ClaranAIM/pkg/config"
	"time"

	"github.com/cloudwego/hertz/pkg/route"
)

// RegisterRoutes 统一注册 api-gateway 暴露给浏览器的 HTTP 路由。
//
// 这个项目采用“HTTP 网关 + 内部 Kitex RPC 服务”的分层：
//   - router 只描述 URL、HTTP 方法和是否需要 JWT；
//   - handler 负责参数绑定、认证用户提取和响应格式；
//   - 具体业务规则在各内部 service 中实现。
//
// 这样前端始终访问 /api/v1/*，不会直接依赖内部微服务端口，也方便后续
// 给网关统一加限流、审计、CORS、鉴权和灰度策略。
func RegisterRoutes(r *route.Engine, cfg ...*config.Config) {
	userHandler := handler.NewUserHandler()
	groupHandler := handler.NewGroupHandler()
	messageHandler := handler.NewMessageHandler()
	fileHandler := handler.NewFileHandler()
	botHandler := handler.NewBotHandler()

	r.Use(middleware.CORSMiddleware())
	if len(cfg) > 0 && cfg[0] != nil {
		r.Use(middleware.RateLimitMiddleware(
			cfg[0].Governance.RateLimit.Enabled,
			cfg[0].Governance.RateLimit.Burst,
			time.Duration(cfg[0].Governance.RateLimit.WindowSeconds)*time.Second,
		))
	}

	public := r.Group("/api/v1")
	{
		public.POST("/user/register", userHandler.Register)
		public.POST("/user/login", userHandler.Login)
		public.POST("/user/token/refresh", userHandler.RefreshToken)
	}

	auth := r.Group("/api/v1")
	auth.Use(middleware.JWTAuthMiddleware())
	{
		// Authenticated REST facade. Handlers keep HTTP concerns here and delegate
		// business rules to Kitex services through internal/api-gateway/client.
		auth.GET("/user/info", userHandler.GetUserInfo)
		auth.PUT("/user/info", userHandler.UpdateUserInfo)
		auth.POST("/user/avatar", userHandler.UpdateAvatar)
		auth.POST("/user/logout", userHandler.Logout)
		auth.POST("/user/friend/add", userHandler.AddFriend)
		auth.POST("/user/friend/delete", userHandler.DeleteFriend)
		auth.PUT("/user/friend/remark", userHandler.UpdateFriendRemark)
		auth.GET("/user/friend/list", userHandler.GetFriendList)
		auth.POST("/user/friend/group", userHandler.CreateFriendGroup)
		auth.GET("/user/friend/groups", userHandler.GetFriendGroups)
		auth.GET("/user/batch", userHandler.BatchGetUserInfo)

		auth.POST("/group/create", groupHandler.CreateGroup)
		auth.GET("/group/:id", groupHandler.GetGroup)
		auth.GET("/group/list", groupHandler.GetUserGroups)
		auth.POST("/group/invite", groupHandler.InviteMember)
		auth.POST("/group/kick", groupHandler.KickMember)
		auth.GET("/group/:id/members", groupHandler.GetGroupMembers)
		auth.POST("/group/transfer", groupHandler.TransferOwner)
		auth.PUT("/group/info", groupHandler.UpdateGroupInfo)
		auth.POST("/group/pin", groupHandler.PinGroup)
		auth.POST("/group/mute", groupHandler.MuteMember)
		auth.POST("/group/unmute", groupHandler.UnmuteMember)
		auth.POST("/group/role", groupHandler.SetRole)
		auth.POST("/group/delete", groupHandler.DeleteGroup)

		auth.POST("/message/conversation", messageHandler.CreateConversation)
		auth.POST("/message/send", messageHandler.SendMessage)
		auth.POST("/message/read", messageHandler.MarkConversationRead)
		// 删除本地消息只影响当前用户视图；全局撤回使用 /message/recall。
		auth.POST("/message/delete-local", messageHandler.DeleteLocalMessage)
		auth.PUT("/message/edit", messageHandler.EditMessage)
		auth.POST("/message/recall", messageHandler.RecallMessage)
		auth.GET("/message/history/:id", messageHandler.GetHistory)
		auth.GET("/message/search", messageHandler.SearchMessages)
		auth.GET("/message/conversations", messageHandler.GetUserConversations)

		auth.POST("/file/upload", fileHandler.UploadFile)
		auth.GET("/file/download/:id", fileHandler.DownloadFile)
		auth.GET("/file/:id", fileHandler.GetFile)
		auth.DELETE("/file/:id", fileHandler.DeleteFile)
		auth.GET("/file/list", fileHandler.ListFiles)

		auth.POST("/bot/create", botHandler.CreateBot)
		auth.PUT("/bot/update", botHandler.UpdateBot)
		auth.GET("/bot/:id", botHandler.GetBot)
		auth.GET("/bot/list", botHandler.ListBots)
		auth.DELETE("/bot/delete", botHandler.DeleteBot)
		auth.POST("/bot/chat", botHandler.ChatWithBot)
		auth.POST("/bot/route/create", botHandler.CreateRoute)
		auth.GET("/bot/:id/routes", botHandler.ListRoutes)
		auth.DELETE("/bot/route/delete", botHandler.DeleteRoute)
		auth.GET("/bot/:id/billing", botHandler.GetBilling)

		auth.POST("/agent/run", botHandler.RunAgent)
		auth.POST("/agent/summarize", botHandler.SummarizeConversation)
		auth.POST("/agent/ask", botHandler.AskConversation)
		auth.POST("/agent/insights", botHandler.ExtractInsights)
		auth.POST("/agent/reply-candidates", botHandler.GenerateReplyCandidates)
		auth.GET("/agent/approvals", botHandler.ListAgentApprovals)
		auth.POST("/agent/approval/confirm", botHandler.ConfirmAgentApproval)
		auth.POST("/agent/approval/reject", botHandler.RejectAgentApproval)
		auth.POST("/agent/add-friend", botHandler.AddAgentAsFriend)
		auth.POST("/agent/permission/grant", botHandler.GrantPermission)
		auth.POST("/agent/permission/revoke", botHandler.RevokePermission)
		auth.GET("/agent/:id/permissions", botHandler.ListPermissions)
		auth.GET("/agent/:id/sessions", botHandler.ListAgentSessions)
	}

	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuthMiddleware(), middleware.RequireRole("admin"))
	{
		// 管理层路由占位：后续新增系统管理接口必须挂在该分组下。
	}

	// Local object preview endpoint for files stored by the gateway. Metadata
	// access remains authenticated through /api/v1/file/*.
	r.GET("/files/*filepath", fileHandler.ServeLocalFile)
	r.GET("/file/preview/:id", fileHandler.PreviewFile)
}
