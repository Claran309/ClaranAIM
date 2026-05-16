package router

import (
	"ClaranAIM/internal/api-gateway/handler"
	"ClaranAIM/internal/api-gateway/middleware"

	"github.com/cloudwego/hertz/pkg/route"
)

func RegisterRoutes(r *route.Engine) {
	userHandler := handler.NewUserHandler()
	groupHandler := handler.NewGroupHandler()
	messageHandler := handler.NewMessageHandler()
	fileHandler := handler.NewFileHandler()
	botHandler := handler.NewBotHandler()

	r.Use(middleware.CORSMiddleware())

	public := r.Group("/api/v1")
	{
		public.POST("/user/register", userHandler.Register)
		public.POST("/user/login", userHandler.Login)
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
	}

	// Local object preview endpoint for files stored by the gateway. Metadata
	// access remains authenticated through /api/v1/file/*.
	r.GET("/files/*filepath", fileHandler.ServeLocalFile)
	r.GET("/file/preview/:id", fileHandler.PreviewFile)
}
