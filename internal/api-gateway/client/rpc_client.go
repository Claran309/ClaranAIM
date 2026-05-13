package client

import (
	"ClaranAIM/kitex_gen/bot"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/kitex_gen/file"
	"ClaranAIM/kitex_gen/file/fileservice"
	"ClaranAIM/kitex_gen/group"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/kitex_gen/message/historyservice"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
	"log"
	"sync"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/transport"
	etcd "github.com/kitex-contrib/registry-etcd"
)

var (
	once          sync.Once
	UserClient    userservice.Client
	GroupClient   groupservice.Client
	MessageClient messageservice.Client
	HistoryClient historyservice.Client
	FileClient    fileservice.Client
	BotClient     botservice.Client
)

func InitClients(etcdEndpoints []string) {
	once.Do(func() {
		r, err := etcd.NewEtcdResolver(etcdEndpoints)
		if err != nil {
			log.Fatal("创建etcd resolver失败:", err)
		}

		UserClient, err = userservice.NewClient("user-service",
			client.WithResolver(r),
			client.WithTransportProtocol(transport.TTHeader),
		)
		if err != nil {
			log.Fatal("创建user-service客户端失败:", err)
		}

		GroupClient, err = groupservice.NewClient("group-service",
			client.WithResolver(r),
			client.WithTransportProtocol(transport.TTHeader),
		)
		if err != nil {
			log.Fatal("创建group-service客户端失败:", err)
		}

		MessageClient, err = messageservice.NewClient("msg-core-service",
			client.WithResolver(r),
			client.WithTransportProtocol(transport.TTHeader),
		)
		if err != nil {
			log.Fatal("创建msg-core-service客户端失败:", err)
		}

		HistoryClient, err = historyservice.NewClient("msg-history-service",
			client.WithResolver(r),
			client.WithTransportProtocol(transport.TTHeader),
		)
		if err != nil {
			log.Fatal("创建msg-history-service客户端失败:", err)
		}

		FileClient, err = fileservice.NewClient("file-service",
			client.WithResolver(r),
			client.WithTransportProtocol(transport.TTHeader),
		)
		if err != nil {
			log.Fatal("创建file-service客户端失败:", err)
		}

		BotClient, err = botservice.NewClient("bot-manager-service",
			client.WithResolver(r),
			client.WithTransportProtocol(transport.TTHeader),
		)
		if err != nil {
			log.Fatal("创建bot-manager-service客户端失败:", err)
		}

		log.Println("RPC客户端初始化成功")
	})
}

func NewRegisterReq(username, password, nickname string) *user.RegisterReq {
	return &user.RegisterReq{Username: username, Password: password, Nickname: nickname}
}

func NewLoginReq(username, password string) *user.LoginReq {
	return &user.LoginReq{Username: username, Password: password}
}

func NewGetUserInfoReq(userID int64) *user.GetUserInfoReq {
	return &user.GetUserInfoReq{UserId: userID}
}

func NewUpdateUserInfoReq(userID int64, nickname, email, phone string) *user.UpdateUserInfoReq {
	return &user.UpdateUserInfoReq{UserId: userID, Nickname: nickname, Email: email, Phone: phone}
}

func NewAddFriendReq(userID, friendID, groupID int64, remark string) *user.AddFriendReq {
	return &user.AddFriendReq{UserId: userID, FriendId: friendID, GroupId: groupID, Remark: remark}
}

func NewDeleteFriendReq(userID, friendID int64) *user.DeleteFriendReq {
	return &user.DeleteFriendReq{UserId: userID, FriendId: friendID}
}

func NewGetFriendListReq(userID int64) *user.GetFriendListReq {
	return &user.GetFriendListReq{UserId: userID}
}

func NewBatchGetUserInfoReq(ids []int64) *user.BatchGetUserInfoReq {
	return &user.BatchGetUserInfoReq{UserIds: ids}
}

func NewCreateGroupReq(name string, ownerID int64, memberIDs []int64) *group.CreateGroupReq {
	return &group.CreateGroupReq{Name: name, OwnerId: ownerID, MemberIds: memberIDs}
}

func NewDeleteGroupReq(groupID, operatorID int64) *group.DeleteGroupReq {
	return &group.DeleteGroupReq{GroupId: groupID, OperatorId: operatorID}
}

func NewGetGroupReq(groupID int64) *group.GetGroupReq {
	return &group.GetGroupReq{GroupId: groupID}
}

func NewGetUserGroupsReq(userID int64) *group.GetUserGroupsReq {
	return &group.GetUserGroupsReq{UserId: userID}
}

func NewUpdateGroupReq(groupID, operatorID int64, name, announcement string) *group.UpdateGroupReq {
	return &group.UpdateGroupReq{GroupId: groupID, OperatorId: operatorID, Name: name, Announcement: announcement}
}

func NewInviteMemberReq(groupID, operatorID int64, userIDs []int64) *group.InviteMemberReq {
	return &group.InviteMemberReq{GroupId: groupID, OperatorId: operatorID, UserIds: userIDs}
}

func NewKickMemberReq(groupID, operatorID, userID int64) *group.KickMemberReq {
	return &group.KickMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID}
}

func NewGetGroupMembersReq(groupID int64) *group.GetGroupMembersReq {
	return &group.GetGroupMembersReq{GroupId: groupID}
}

func NewTransferOwnerReq(groupID, operatorID, newOwnerID int64) *group.TransferOwnerReq {
	return &group.TransferOwnerReq{GroupId: groupID, OperatorId: operatorID, NewOwnerId_: newOwnerID}
}

func NewPinGroupReq(groupID, operatorID int64, isPinned bool) *group.PinGroupReq {
	return &group.PinGroupReq{GroupId: groupID, OperatorId: operatorID, IsPinned: isPinned}
}

func NewMuteMemberReq(groupID, operatorID, userID, durationMinutes int64) *group.MuteMemberReq {
	return &group.MuteMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID, DurationMinutes: durationMinutes}
}

func NewUnmuteMemberReq(groupID, operatorID, userID int64) *group.UnmuteMemberReq {
	return &group.UnmuteMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID}
}

func NewSetRoleReq(groupID, operatorID, userID int64, role string) *group.SetRoleReq {
	return &group.SetRoleReq{GroupId: groupID, OperatorId: operatorID, UserId: userID, Role: role}
}

func NewCheckMemberReq(groupID, userID int64) *group.CheckMemberReq {
	return &group.CheckMemberReq{GroupId: groupID, UserId: userID}
}

func NewCreateConversationReq(convType string, participantIDs []int64, groupID int64) *message.CreateConversationReq {
	return &message.CreateConversationReq{Type: convType, ParticipantIds: participantIDs, GroupId: groupID}
}

func NewSendMessageReq(conversationID, senderID int64, content, msgType string) *message.SendMessageReq {
	return &message.SendMessageReq{ConversationId: conversationID, SenderId: senderID, Content: content, MsgType: msgType}
}

func NewGetHistoryReq(conversationID, userID, limit, beforeID int64) *message.GetHistoryReq {
	return &message.GetHistoryReq{ConversationId: conversationID, UserId: userID, Limit: limit, BeforeId: beforeID}
}

func NewSearchMessagesReq(userID int64, keyword string, limit int64) *message.SearchMessagesReq {
	return &message.SearchMessagesReq{UserId: userID, Keyword: keyword, Limit: limit}
}

func NewSearchMessagesInConvReq(conversationIDs []int64, keyword string, limit int64) *message.SearchMessagesReq {
	return &message.SearchMessagesReq{ConversationIds: conversationIDs, Keyword: keyword, Limit: limit}
}

func NewUploadFileReq(fileName, fileType string, fileSize int64, contentType string, uploaderID int64) *file.UploadFileReq {
	return &file.UploadFileReq{FileName: fileName, FileType: fileType, FileSize: fileSize, ContentType: contentType, UploaderId: uploaderID}
}

func NewGetFileReq(fileID string) *file.GetFileReq {
	return &file.GetFileReq{FileId: fileID}
}

func NewDeleteFileReq(fileID string, operatorID int64) *file.DeleteFileReq {
	return &file.DeleteFileReq{FileId: fileID, OperatorId: operatorID}
}

func NewListFilesReq(uploaderID int64, fileType string, limit, offset int64) *file.ListFilesReq {
	return &file.ListFilesReq{UploaderId: uploaderID, FileType: fileType, Limit: limit, Offset: offset}
}

func NewCreateBotReq(name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot string, ownerID int64) *bot.CreateBotReq {
	return &bot.CreateBotReq{Name: name, Type: botType, Description: description, ModelName: modelName, ApiKey: apiKey, BaseUrl: baseURL, SystemPrompt: systemPrompt, SkillsDir: skillsDir, AgentRoot: agentRoot, OwnerId: ownerID}
}

func NewUpdateBotReq(botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot string, isActive bool) *bot.UpdateBotReq {
	return &bot.UpdateBotReq{BotId: botID, OperatorId: operatorID, Name: name, Description: description, ModelName: modelName, ApiKey: apiKey, BaseUrl: baseURL, SystemPrompt: systemPrompt, SkillsDir: skillsDir, AgentRoot: agentRoot, IsActive: isActive}
}

func NewGetBotReq(botID int64) *bot.GetBotReq {
	return &bot.GetBotReq{BotId: botID}
}

func NewListBotsReq(ownerID int64, botType string) *bot.ListBotsReq {
	return &bot.ListBotsReq{OwnerId: ownerID, Type: botType}
}

func NewDeleteBotReq(botID, operatorID int64) *bot.DeleteBotReq {
	return &bot.DeleteBotReq{BotId: botID, OperatorId: operatorID}
}

func NewCreateRouteReq(botID int64, routePattern, routeType string, priority int64) *bot.CreateRouteReq {
	return &bot.CreateRouteReq{BotId: botID, RoutePattern: routePattern, RouteType: routeType, Priority: priority}
}

func NewListRoutesReq(botID int64) *bot.ListRoutesReq {
	return &bot.ListRoutesReq{BotId: botID}
}

func NewDeleteRouteReq(routeID, operatorID int64) *bot.DeleteRouteReq {
	return &bot.DeleteRouteReq{RouteId: routeID, OperatorId: operatorID}
}

func NewGetBillingReq(botID, userID, limit, offset int64) *bot.GetBillingReq {
	return &bot.GetBillingReq{BotId: botID, UserId: userID, Limit: limit, Offset: offset}
}

func NewChatWithBotReq(botID, userID, conversationID int64, message string) *bot.ChatWithBotReq {
	return &bot.ChatWithBotReq{BotId: botID, UserId: userID, ConversationId: conversationID, Message: message}
}
