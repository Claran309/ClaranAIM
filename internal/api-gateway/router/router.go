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
	webSearchHandler := handler.NewWebSearchHandler()
	conversationIntelligenceHandler := handler.NewConversationIntelligenceHandler()
	mcpHandler := handler.NewMCPHandler()
	adminHandler := handler.NewAdminHandler()

	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.ObservabilityMiddleware())
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
		auth.GET("/message/offline", messageHandler.GetOfflineMessages)
		auth.POST("/message/offline/read", messageHandler.MarkOfflineRead)
		auth.GET("/message/unread-count", messageHandler.GetUnreadCount)
		auth.GET("/message/sync", messageHandler.SyncOnReconnect)
		auth.POST("/message/sync/ack", messageHandler.AckSync)
		auth.POST("/message/translate", messageHandler.TranslateMessage)
		auth.GET("/system/notices", adminHandler.ListPublicNotices)

		auth.POST("/file/upload", fileHandler.UploadFile)
		auth.POST("/file/:id/ocr", fileHandler.AnalyzeImage)
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
		auth.POST("/agent/:id/skill/smoke-test", agentHandler.SmokeTestSkill)
		auth.GET("/agent/:id", agentHandler.GetAgent)
		auth.GET("/agent/:id/routes", agentHandler.ListRoutes)
		auth.GET("/agent/:id/billing", agentHandler.GetBilling)
		auth.GET("/agent/:id/permissions", agentHandler.ListPermissions)
		auth.GET("/agent/:id/sessions", agentHandler.ListAgentSessions)

		auth.GET("/memory/list", memoryHandler.ListMemories)
		auth.POST("/memory/create", memoryHandler.CreateMemory)
		auth.PUT("/memory/:id", memoryHandler.UpdateMemory)
		auth.DELETE("/memory/:id", memoryHandler.DeleteMemory)
		auth.GET("/memory/candidates", memoryHandler.ListCandidates)
		auth.POST("/memory/candidate/create", memoryHandler.CreateCandidate)
		auth.POST("/memory/candidate/:id/accept", memoryHandler.AcceptCandidate)
		auth.POST("/memory/candidate/:id/reject", memoryHandler.RejectCandidate)

		auth.POST("/rag/ingest", ragHandler.IngestDocument)
		auth.POST("/rag/upload", ragHandler.UploadDocument)
		auth.GET("/rag/upload/:id", ragHandler.GetUploadJob)
		auth.POST("/rag/upload/:id/retry", ragHandler.RetryUploadJob)
		auth.DELETE("/rag/upload/:id", ragHandler.CancelUploadJob)
		auth.DELETE("/rag/upload", ragHandler.CancelAllUploadJobs)
		auth.POST("/rag/search", ragHandler.Search)
		auth.GET("/rag/graph", ragHandler.GetGraph)
		auth.POST("/rag/graph/rebuild", ragHandler.RebuildAllGraphs)
		auth.GET("/rag/documents", ragHandler.ListDocuments)
		auth.DELETE("/rag/documents/:id", ragHandler.DeleteDocument)
		auth.DELETE("/rag/documents/:id/graph", ragHandler.DeleteDocumentGraph)
		auth.POST("/rag/documents/:id/graph/rebuild", ragHandler.RebuildDocumentGraph)

		auth.GET("/web-search/search", webSearchHandler.Search)
		auth.POST("/web-search/augment", webSearchHandler.Augment)

		auth.POST("/conversation-intelligence/jobs", conversationIntelligenceHandler.CreateDigestJob)
		auth.GET("/conversation-intelligence/jobs", conversationIntelligenceHandler.ListDigestJobs)
		auth.POST("/conversation-intelligence/jobs/:id/process", conversationIntelligenceHandler.ProcessDigestJob)
		auth.POST("/conversation-intelligence/jobs/:id/retry", conversationIntelligenceHandler.RetryDigestJob)
		auth.GET("/conversation-intelligence/artifacts", conversationIntelligenceHandler.ListArtifacts)
		auth.POST("/conversation-intelligence/missed-summary", conversationIntelligenceHandler.MissedSummary)

		auth.GET("/knowledge/graph", knowledgeHandler.GetGraphView)
		auth.GET("/knowledge/node/:id", knowledgeHandler.GetNodeDetail)
		auth.GET("/knowledge/node/:id/neighborhood", knowledgeHandler.GetNeighborhood)
		auth.GET("/knowledge/edge/:id", knowledgeHandler.GetEdgeDetail)
		auth.GET("/knowledge/path", knowledgeHandler.GetPath)
		auth.GET("/knowledge/review-candidates", knowledgeHandler.ListReviewCandidates)
		auth.POST("/knowledge/review-candidates", knowledgeHandler.CreateReviewCandidate)
		auth.POST("/knowledge/review-candidates/:id/review", knowledgeHandler.ReviewCandidate)

		auth.GET("/mcp/tools", mcpHandler.ListTools)
		auth.POST("/mcp/call", mcpHandler.CallTool)
		auth.GET("/mcp/traces", mcpHandler.ListToolCalls)
		auth.GET("/mcp/traces/:trace_id", mcpHandler.GetToolCallTrace)

		auth.GET("/settings/llm-profiles", settingsHandler.ListLLMProfiles)
		auth.POST("/settings/llm-profiles", settingsHandler.SaveLLMProfile)
		auth.POST("/settings/llm-profiles/test", settingsHandler.TestLLMProfile)
		auth.DELETE("/settings/llm-profiles/:id", settingsHandler.DeleteLLMProfile)
		auth.GET("/settings/prompts", settingsHandler.ListPrompts)
		auth.POST("/settings/prompts", settingsHandler.SavePrompt)
		auth.GET("/settings/skills", settingsHandler.ListSkills)
		auth.POST("/settings/skills/upload", settingsHandler.UploadSkill)
		auth.GET("/settings/skills/:id", settingsHandler.GetSkill)
		auth.PUT("/settings/skills/:id", settingsHandler.UpdateSkillContent)
		auth.DELETE("/settings/skills/:id", settingsHandler.DeleteSkill)
		auth.GET("/settings/mcp-servers", settingsHandler.ListMCPServers)
		auth.POST("/settings/mcp-servers", settingsHandler.SaveMCPServer)
		auth.DELETE("/settings/mcp-servers/:id", settingsHandler.DeleteMCPServer)
	}

	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuthMiddleware(), middleware.RequireRole("admin"))
	{
		admin.GET("/dashboard", adminHandler.Dashboard)
		admin.GET("/users", adminHandler.ListUsers)
		admin.POST("/users/:id/status", adminHandler.UpdateUserStatus)
		admin.POST("/users/:id/role", adminHandler.UpdateUserRole)
		admin.GET("/groups", adminHandler.ListGroups)
		admin.POST("/groups/:id/status", adminHandler.UpdateGroupStatus)
		admin.GET("/files", adminHandler.ListFiles)
		admin.GET("/agents", adminHandler.ListAgents)
		admin.GET("/billing", adminHandler.ListBilling)
		admin.GET("/reviews", adminHandler.ListReviews)
		admin.POST("/reviews/action", adminHandler.ReviewItem)
		admin.GET("/mcp/traces", adminHandler.ListMCPTraces)
		admin.GET("/observability/links", adminHandler.ObservabilityLinks)
		admin.GET("/notices", adminHandler.ListNotices)
		admin.POST("/notices", adminHandler.SaveNotice)
		admin.GET("/audits", adminHandler.ListAuditLogs)
	}

	// 网关本地存储文件的预览端点；元数据仍由 file-service 管理。
	// access remains authenticated through /api/v1/file/*.
	r.GET("/files/*filepath", fileHandler.ServeLocalFile)
	r.GET("/file/preview/:id", fileHandler.PreviewFile)
}
