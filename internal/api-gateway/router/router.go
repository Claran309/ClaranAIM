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
	agentHandler := handler.NewAgentHandler()
	memoryHandler := handler.NewMemoryHandler()
	ragHandler := handler.NewRAGHandler()
	knowledgeHandler := handler.NewKnowledgeHandler()
	settingsHandler := handler.NewSettingsHandler()

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
		// 需要登录的 REST 门面。handler 只处理 HTTP 绑定和登录态，业务规则交给内部 Kitex 服务。
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
		auth.GET("/group/list", groupHandler.GetUserGroups)
		auth.POST("/group/join", groupHandler.JoinGroupByID)
		auth.POST("/group/invite", groupHandler.InviteMember)
		auth.POST("/group/kick", groupHandler.KickMember)
		auth.POST("/group/transfer", groupHandler.TransferOwner)
		auth.PUT("/group/info", groupHandler.UpdateGroupInfo)
		auth.POST("/group/pin", groupHandler.PinGroup)
		auth.POST("/group/mute", groupHandler.MuteMember)
		auth.POST("/group/unmute", groupHandler.UnmuteMember)
		auth.POST("/group/role", groupHandler.SetRole)
		auth.POST("/group/delete", groupHandler.DeleteGroup)
		auth.GET("/group/:id", groupHandler.GetGroup)
		auth.GET("/group/:id/members", groupHandler.GetGroupMembers)

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
		auth.POST("/message/translate", messageHandler.TranslateMessage)

		auth.POST("/file/upload", fileHandler.UploadFile)
		auth.GET("/file/download/:id", fileHandler.DownloadFile)
		auth.GET("/file/list", fileHandler.ListFiles)
		auth.GET("/file/:id", fileHandler.GetFile)
		auth.DELETE("/file/:id", fileHandler.DeleteFile)

		auth.POST("/agent/create", agentHandler.CreateAgent)
		auth.PUT("/agent/update", agentHandler.UpdateAgent)
		auth.GET("/agent/list", agentHandler.ListAgents)
		auth.DELETE("/agent/delete", agentHandler.DeleteAgent)
		auth.POST("/agent/chat", agentHandler.ChatWithAgent)
		auth.POST("/agent/route/create", agentHandler.CreateRoute)
		auth.DELETE("/agent/route/delete", agentHandler.DeleteRoute)
		auth.POST("/agent/run", agentHandler.RunAgent)
		auth.POST("/agent/summarize", agentHandler.SummarizeConversation)
		auth.POST("/agent/ask", agentHandler.AskConversation)
		auth.POST("/agent/insights", agentHandler.ExtractInsights)
		auth.POST("/agent/reply-candidates", agentHandler.GenerateReplyCandidates)
		auth.GET("/agent/approvals", agentHandler.ListAgentApprovals)
		auth.POST("/agent/approval/confirm", agentHandler.ConfirmAgentApproval)
		auth.POST("/agent/approval/reject", agentHandler.RejectAgentApproval)
		auth.POST("/agent/add-friend", agentHandler.AddAgentAsFriend)
		auth.POST("/agent/permission/grant", agentHandler.GrantPermission)
		auth.POST("/agent/permission/revoke", agentHandler.RevokePermission)
		auth.GET("/agent/:id", agentHandler.GetAgent)
		auth.GET("/agent/:id/routes", agentHandler.ListRoutes)
		auth.GET("/agent/:id/billing", agentHandler.GetBilling)
		auth.GET("/agent/:id/permissions", agentHandler.ListPermissions)
		auth.GET("/agent/:id/sessions", agentHandler.ListAgentSessions)

		auth.GET("/memory/list", memoryHandler.ListMemories)
		auth.POST("/memory/create", memoryHandler.CreateMemory)
		auth.PUT("/memory/:id", memoryHandler.UpdateMemory)
		auth.DELETE("/memory/:id", memoryHandler.DeleteMemory)

		auth.POST("/rag/ingest", ragHandler.IngestDocument)
		auth.POST("/rag/upload", ragHandler.UploadDocument)
		auth.POST("/rag/search", ragHandler.Search)
		auth.GET("/rag/graph", ragHandler.GetGraph)
		auth.GET("/rag/documents", ragHandler.ListDocuments)

		auth.GET("/knowledge/graph", knowledgeHandler.GetGraphView)
		auth.GET("/knowledge/node/:id", knowledgeHandler.GetNodeDetail)
		auth.GET("/knowledge/node/:id/neighborhood", knowledgeHandler.GetNeighborhood)
		auth.GET("/knowledge/edge/:id", knowledgeHandler.GetEdgeDetail)
		auth.GET("/knowledge/path", knowledgeHandler.GetPath)

		auth.GET("/settings/llm-profiles", settingsHandler.ListLLMProfiles)
		auth.POST("/settings/llm-profiles", settingsHandler.SaveLLMProfile)
		auth.DELETE("/settings/llm-profiles/:id", settingsHandler.DeleteLLMProfile)
		auth.GET("/settings/prompts", settingsHandler.ListPrompts)
		auth.POST("/settings/prompts", settingsHandler.SavePrompt)
		auth.GET("/settings/skills", settingsHandler.ListSkills)
		auth.POST("/settings/skills/upload", settingsHandler.UploadSkill)
		auth.GET("/settings/skills/:id", settingsHandler.GetSkill)
		auth.PUT("/settings/skills/:id", settingsHandler.UpdateSkillContent)
		auth.DELETE("/settings/skills/:id", settingsHandler.DeleteSkill)
	}

	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuthMiddleware(), middleware.RequireRole("admin"))
	{
		// 管理层路由占位：后续新增系统管理接口必须挂在该分组下。
	}

	// 网关本地存储文件的预览端点；元数据仍由 file-service 管理。
	// access remains authenticated through /api/v1/file/*.
	r.GET("/files/*filepath", fileHandler.ServeLocalFile)
	r.GET("/file/preview/:id", fileHandler.PreviewFile)
}
