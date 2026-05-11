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

	// 全局中间件
	r.Use(middleware.CORSMiddleware())

	// 公开接口（无需认证）
	public := r.Group("/api/v1")
	{
		public.POST("/user/register", userHandler.Register)
		public.POST("/user/login", userHandler.Login)
	}

	// 需要认证的接口
	auth := r.Group("/api/v1")
	auth.Use(middleware.JWTAuthMiddleware())
	{
		// 用户相关
		auth.GET("/user/info", userHandler.GetUserInfo)
		auth.PUT("/user/info", userHandler.UpdateUserInfo)
		auth.POST("/user/friend/add", userHandler.AddFriend)
		auth.POST("/user/friend/delete", userHandler.DeleteFriend)
		auth.GET("/user/friend/list", userHandler.GetFriendList)
		auth.POST("/user/friend/group", userHandler.CreateFriendGroup)
		auth.GET("/user/friend/groups", userHandler.GetFriendGroups)

		// 群组相关
		auth.POST("/group/create", groupHandler.CreateGroup)
		auth.GET("/group/:id", groupHandler.GetGroup)
		auth.GET("/group/list", groupHandler.GetUserGroups)
		auth.POST("/group/invite", groupHandler.InviteMember)
		auth.POST("/group/kick", groupHandler.KickMember)
		auth.GET("/group/:id/members", groupHandler.GetGroupMembers)

		// 消息相关
		auth.POST("/message/conversation", messageHandler.CreateConversation)
		auth.POST("/message/send", messageHandler.SendMessage)
		auth.GET("/message/history/:id", messageHandler.GetHistory)
		auth.GET("/message/search", messageHandler.SearchMessages)
		auth.GET("/message/conversations", messageHandler.GetUserConversations)
	}
}
