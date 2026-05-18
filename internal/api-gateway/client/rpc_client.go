// Package client holds the api-gateway side of all internal RPC dependencies.
//
// The gateway translates browser HTTP requests into Kitex RPC calls. Keeping the
// generated request construction in this package gives handlers a small, stable
// surface and prevents HTTP binding details from leaking into service clients.
package client

import (
	"ClaranAIM/kitex_gen/bot"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"ClaranAIM/kitex_gen/file"
	"ClaranAIM/kitex_gen/file/fileservice"
	"ClaranAIM/kitex_gen/group"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/kitex_gen/message/historyservice"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"log"
	"sync"

	"github.com/cloudwego/kitex/client"
	etcd "github.com/kitex-contrib/registry-etcd"
)

var (
	once sync.Once

	// UserClient calls user-service for accounts, profiles, friends and online
	// status. It is initialized once by InitClients during api-gateway startup.
	UserClient userservice.Client
	// GroupClient calls group-service for group metadata, membership and roles.
	GroupClient groupservice.Client
	// MessageClient calls msg-core-service for conversations, message writes,
	// read receipts and local message state.
	MessageClient messageservice.Client
	// HistoryClient is kept for the history-service RPC contract. Most current
	// history reads go through msg-core-service so per-user visibility rules stay
	// in one place.
	HistoryClient historyservice.Client
	// FileClient calls file-service for file metadata after the gateway streams
	// binary content to local storage or MinIO.
	FileClient fileservice.Client
	// BotClient calls bot-manager-service for bot CRUD, bot chat, routing and
	// billing records.
	BotClient botservice.Client
	// BotRuntimeClient calls bot-runtime-service for runtime-owned data that is
	// not part of bot-manager-service's Thrift facade, such as session metadata.
	BotRuntimeClient botruntimeservice.Client
)

// InitClients 初始化 api-gateway 到各内部 Kitex 服务的客户端。
//
// api-gateway 不直接访问各服务数据库，而是通过 Etcd 发现服务地址，再用
// TTHeader 协议调用内部 RPC。once.Do 保证整个进程只初始化一次，避免重复创建
// 连接池和重复注册服务发现监听。
func InitClients(etcdEndpoints []string, rpcCfg ...config.RPCGovernanceConfig) {
	once.Do(func() {
		r, err := etcd.NewEtcdResolver(etcdEndpoints)
		if err != nil {
			log.Fatal("创建etcd resolver失败:", err)
		}

		var cfg config.RPCGovernanceConfig
		if len(rpcCfg) > 0 {
			cfg = rpcCfg[0]
		}
		baseOptions := append([]client.Option{client.WithResolver(r)}, governance.ClientOptions(cfg)...)

		UserClient, err = userservice.NewClient("user-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建user-service客户端失败:", err)
		}

		GroupClient, err = groupservice.NewClient("group-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建group-service客户端失败:", err)
		}

		MessageClient, err = messageservice.NewClient("msg-core-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建msg-core-service客户端失败:", err)
		}

		HistoryClient, err = historyservice.NewClient("msg-history-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建msg-history-service客户端失败:", err)
		}

		FileClient, err = fileservice.NewClient("file-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建file-service客户端失败:", err)
		}

		BotClient, err = botservice.NewClient("bot-manager-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建bot-manager-service客户端失败:", err)
		}

		BotRuntimeClient, err = botruntimeservice.NewClient("bot-runtime-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建bot-runtime-service客户端失败:", err)
		}

		log.Println("RPC客户端初始化成功")
	})
}

// NewRegisterReq builds a user-service register request from HTTP form fields.
func NewRegisterReq(username, password, nickname string) *user.RegisterReq {
	return &user.RegisterReq{Username: username, Password: password, Nickname: nickname}
}

// NewRegisterSystemUserReq creates a non-login account for internal actors such
// as Agents. Normal browser registration must keep using NewRegisterReq.
func NewRegisterSystemUserReq(username, password, nickname string) *user.RegisterReq {
	return &user.RegisterReq{Username: username, Password: password, Nickname: nickname, IsSystem: true}
}

// NewLoginReq builds a user-service login request. Password verification stays
// inside user-service; the gateway never hashes or stores passwords.
func NewLoginReq(username, password string) *user.LoginReq {
	return &user.LoginReq{Username: username, Password: password}
}

// NewGetUserInfoReq builds the profile lookup request used by profile pages,
// friend lists and message sender-name hydration.
func NewGetUserInfoReq(userID int64) *user.GetUserInfoReq {
	return &user.GetUserInfoReq{UserId: userID}
}

// NewUpdateUserInfoReq builds the profile update request. fullUpdate tells
// user-service whether empty strings are intentional clears or should be ignored.
func NewUpdateUserInfoReq(
	userID int64,
	nickname, email, phone, avatar, cover, signature, bio, location, website, gender, birthday string,
	fullUpdate bool,
) *user.UpdateUserInfoReq {
	return &user.UpdateUserInfoReq{
		UserId:     userID,
		Nickname:   nickname,
		Email:      email,
		Phone:      phone,
		Avatar:     avatar,
		Cover:      cover,
		Signature:  signature,
		Bio:        bio,
		Location:   location,
		Website:    website,
		Gender:     gender,
		Birthday:   birthday,
		FullUpdate: fullUpdate,
	}
}

// NewAddFriendReq builds the friend-add request. userID is always the current
// authenticated user, never a client-supplied value.
func NewAddFriendReq(userID, friendID, groupID int64, remark string) *user.AddFriendReq {
	return &user.AddFriendReq{UserId: userID, FriendId: friendID, GroupId: groupID, Remark: remark}
}

// NewDeleteFriendReq builds the symmetric friend-delete request.
func NewDeleteFriendReq(userID, friendID int64) *user.DeleteFriendReq {
	return &user.DeleteFriendReq{UserId: userID, FriendId: friendID}
}

// NewUpdateFriendRemarkReq builds a request for remark/group changes on one
// friend relation owned by the current user.
func NewUpdateFriendRemarkReq(userID, friendID, groupID int64, remark string) *user.UpdateFriendRemarkReq {
	return &user.UpdateFriendRemarkReq{UserId: userID, FriendId: friendID, Remark: remark, GroupId: groupID}
}

// NewGetFriendListReq builds the friend-list lookup request for the sidebar.
func NewGetFriendListReq(userID int64) *user.GetFriendListReq {
	return &user.GetFriendListReq{UserId: userID}
}

// NewBatchGetUserInfoReq builds a batch profile request used to hydrate message
// sender names and avatars without issuing one RPC per visible message.
func NewBatchGetUserInfoReq(ids []int64) *user.BatchGetUserInfoReq {
	return &user.BatchGetUserInfoReq{UserIds: ids}
}

// The New...Req helpers below intentionally contain no business rules. They are
// narrow adapters from handler-validated primitive values into generated Thrift
// structs. Permission checks, membership checks and persistence all remain in
// the owning microservice.

// NewCreateGroupReq builds a group-service create request.
func NewCreateGroupReq(name string, ownerID int64, memberIDs []int64) *group.CreateGroupReq {
	return &group.CreateGroupReq{Name: name, OwnerId: ownerID, MemberIds: memberIDs}
}

// NewDeleteGroupReq builds a group deletion request with the current user as operator.
func NewDeleteGroupReq(groupID, operatorID int64) *group.DeleteGroupReq {
	return &group.DeleteGroupReq{GroupId: groupID, OperatorId: operatorID}
}

// NewGetGroupReq builds a group metadata lookup request.
func NewGetGroupReq(groupID int64) *group.GetGroupReq {
	return &group.GetGroupReq{GroupId: groupID}
}

// NewGetUserGroupsReq builds the current user's group-list request.
func NewGetUserGroupsReq(userID int64) *group.GetUserGroupsReq {
	return &group.GetUserGroupsReq{UserId: userID}
}

// NewUpdateGroupReq builds a group metadata update request. An empty announcement
// is meaningful and means "clear announcement" after the gateway has confirmed
// the JSON field was explicitly present.
func NewUpdateGroupReq(groupID, operatorID int64, name, announcement string) *group.UpdateGroupReq {
	return &group.UpdateGroupReq{GroupId: groupID, OperatorId: operatorID, Name: name, Announcement: announcement}
}

// NewInviteMemberReq builds a group member invite request.
func NewInviteMemberReq(groupID, operatorID int64, userIDs []int64) *group.InviteMemberReq {
	return &group.InviteMemberReq{GroupId: groupID, OperatorId: operatorID, UserIds: userIDs}
}

// NewKickMemberReq builds a group member removal request.
func NewKickMemberReq(groupID, operatorID, userID int64) *group.KickMemberReq {
	return &group.KickMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID}
}

// NewGetGroupMembersReq builds a group member list request.
func NewGetGroupMembersReq(groupID int64) *group.GetGroupMembersReq {
	return &group.GetGroupMembersReq{GroupId: groupID}
}

// NewTransferOwnerReq builds a group ownership transfer request.
func NewTransferOwnerReq(groupID, operatorID, newOwnerID int64) *group.TransferOwnerReq {
	return &group.TransferOwnerReq{GroupId: groupID, OperatorId: operatorID, NewOwnerId_: newOwnerID}
}

// NewPinGroupReq builds a per-user group pin/unpin request.
func NewPinGroupReq(groupID, operatorID int64, isPinned bool) *group.PinGroupReq {
	return &group.PinGroupReq{GroupId: groupID, OperatorId: operatorID, IsPinned: isPinned}
}

// NewMuteMemberReq builds a timed mute request for group moderation.
func NewMuteMemberReq(groupID, operatorID, userID, durationMinutes int64) *group.MuteMemberReq {
	return &group.MuteMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID, DurationMinutes: durationMinutes}
}

// NewUnmuteMemberReq builds a group unmute request.
func NewUnmuteMemberReq(groupID, operatorID, userID int64) *group.UnmuteMemberReq {
	return &group.UnmuteMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID}
}

// NewSetRoleReq builds a group role update request.
func NewSetRoleReq(groupID, operatorID, userID int64, role string) *group.SetRoleReq {
	return &group.SetRoleReq{GroupId: groupID, OperatorId: operatorID, UserId: userID, Role: role}
}

// NewCheckMemberReq builds a group membership check request.
func NewCheckMemberReq(groupID, userID int64) *group.CheckMemberReq {
	return &group.CheckMemberReq{GroupId: groupID, UserId: userID}
}

// NewCreateConversationReq builds a msg-core-service conversation request.
func NewCreateConversationReq(convType string, participantIDs []int64, groupID int64) *message.CreateConversationReq {
	return &message.CreateConversationReq{Type: convType, ParticipantIds: participantIDs, GroupId: groupID}
}

// NewSendMessageReq builds a plain message send request.
func NewSendMessageReq(conversationID, senderID int64, content, msgType string) *message.SendMessageReq {
	return NewSendMessageExtReq(conversationID, senderID, content, msgType, 0, nil, false)
}

// NewSendMessageExtReq builds a message send request carrying reply and mention
// metadata. It is used by normal text, media references and broadcast messages.
func NewSendMessageExtReq(conversationID, senderID int64, content, msgType string, replyToID int64, mentionUserIDs []int64, mentionAll bool) *message.SendMessageReq {
	return NewSendMessageExtReqWithClientID(conversationID, senderID, content, msgType, replyToID, mentionUserIDs, mentionAll, "")
}

// NewSendMessageExtReqWithClientID adds an optional idempotency key. Internal
// producers such as Agent dispatchers should pass a stable key derived from the
// source event so Kafka retries do not create duplicate replies.
func NewSendMessageExtReqWithClientID(conversationID, senderID int64, content, msgType string, replyToID int64, mentionUserIDs []int64, mentionAll bool, clientMsgID string) *message.SendMessageReq {
	return &message.SendMessageReq{
		ConversationId: conversationID,
		SenderId:       senderID,
		Content:        content,
		MsgType:        msgType,
		ReplyToId:      replyToID,
		MentionUserIds: mentionUserIDs,
		MentionAll:     mentionAll,
		ClientMsgId:    clientMsgID,
	}
}

// NewGetHistoryReq builds a history query for one user's visible message view.
func NewGetHistoryReq(conversationID, userID, limit, beforeID int64) *message.GetHistoryReq {
	return &message.GetHistoryReq{ConversationId: conversationID, UserId: userID, Limit: limit, BeforeId: beforeID}
}

// NewDeleteLocalMessageReq builds a request that hides one message only for the
// current user, leaving the global message fact intact.
func NewDeleteLocalMessageReq(conversationID, userID, messageID int64) *message.DeleteLocalMessageReq {
	return &message.DeleteLocalMessageReq{ConversationId: conversationID, UserId: userID, MessageId: messageID}
}

// NewSearchMessagesReq builds a user-scoped history search request.
func NewSearchMessagesReq(userID int64, keyword string, limit int64) *message.SearchMessagesReq {
	return &message.SearchMessagesReq{UserId: userID, Keyword: keyword, Limit: limit}
}

// NewSearchMessagesInConvReq builds a conversation-scoped search request.
func NewSearchMessagesInConvReq(conversationIDs []int64, keyword string, limit int64) *message.SearchMessagesReq {
	return &message.SearchMessagesReq{ConversationIds: conversationIDs, Keyword: keyword, Limit: limit}
}

// NewSearchMessagesAdvancedReq builds a search request with optional time bounds.
func NewSearchMessagesAdvancedReq(userID int64, conversationIDs []int64, keyword string, limit int64, startAt, endAt string) *message.SearchMessagesReq {
	return &message.SearchMessagesReq{UserId: userID, ConversationIds: conversationIDs, Keyword: keyword, Limit: limit, StartAt: startAt, EndAt: endAt}
}

// NewUploadFileReq builds a metadata write request after the gateway has stored
// the binary object in local storage or MinIO.
func NewUploadFileReq(fileName, fileType string, fileSize int64, contentType, fileURL string, uploaderID int64) *file.UploadFileReq {
	return &file.UploadFileReq{FileName: fileName, FileType: fileType, FileSize: fileSize, ContentType: contentType, FileUrl: fileURL, UploaderId: uploaderID}
}

// NewGetFileReq builds a file metadata lookup request.
func NewGetFileReq(fileID string) *file.GetFileReq {
	return &file.GetFileReq{FileId: fileID}
}

// NewDeleteFileReq builds a metadata deletion request with an operator identity.
func NewDeleteFileReq(fileID string, operatorID int64) *file.DeleteFileReq {
	return &file.DeleteFileReq{FileId: fileID, OperatorId: operatorID}
}

// NewListFilesReq builds a paginated file metadata list request.
func NewListFilesReq(uploaderID int64, fileType string, limit, offset int64) *file.ListFilesReq {
	return &file.ListFilesReq{UploaderId: uploaderID, FileType: fileType, Limit: limit, Offset: offset}
}

// NewCreateBotReq builds a bot creation request with owner and model settings.
func NewCreateBotReq(name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, ownerID int64) *bot.CreateBotReq {
	return &bot.CreateBotReq{Name: name, Type: botType, Description: description, ModelName: modelName, ApiKey: apiKey, BaseUrl: baseURL, SystemPrompt: systemPrompt, SkillsDir: skillsDir, AgentRoot: agentRoot, Avatar: avatar, Signature: signature, WorkspaceRoot: workspaceRoot, ToolPolicy: toolPolicy, OwnerId: ownerID}
}

// NewUpdateBotReq builds a bot update request. Empty secrets are interpreted by
// bot-manager-service, not by the gateway.
func NewUpdateBotReq(botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, isActive bool, isActiveSet bool) *bot.UpdateBotReq {
	return &bot.UpdateBotReq{BotId: botID, OperatorId: operatorID, Name: name, Description: description, ModelName: modelName, ApiKey: apiKey, BaseUrl: baseURL, SystemPrompt: systemPrompt, SkillsDir: skillsDir, AgentRoot: agentRoot, Avatar: avatar, Signature: signature, WorkspaceRoot: workspaceRoot, ToolPolicy: toolPolicy, IsActive: isActive, IsActiveSet: isActiveSet}
}

// NewGetBotReq builds a bot metadata lookup request.
func NewGetBotReq(botID int64) *bot.GetBotReq {
	return &bot.GetBotReq{BotId: botID}
}

// NewListBotsReq builds a user-scoped bot list request.
func NewListBotsReq(ownerID int64, botType string) *bot.ListBotsReq {
	return &bot.ListBotsReq{OwnerId: ownerID, Type: botType}
}

// NewDeleteBotReq builds a bot deletion request with an operator identity.
func NewDeleteBotReq(botID, operatorID int64) *bot.DeleteBotReq {
	return &bot.DeleteBotReq{BotId: botID, OperatorId: operatorID}
}

// NewCreateRouteReq builds a bot routing rule creation request.
func NewCreateRouteReq(botID int64, routePattern, routeType string, priority int64) *bot.CreateRouteReq {
	return &bot.CreateRouteReq{BotId: botID, RoutePattern: routePattern, RouteType: routeType, Priority: priority}
}

// NewListRoutesReq builds a bot route list request.
func NewListRoutesReq(botID int64) *bot.ListRoutesReq {
	return &bot.ListRoutesReq{BotId: botID}
}

// NewDeleteRouteReq builds a bot route deletion request.
func NewDeleteRouteReq(routeID, operatorID int64) *bot.DeleteRouteReq {
	return &bot.DeleteRouteReq{RouteId: routeID, OperatorId: operatorID}
}

// NewGetBillingReq builds a paginated billing lookup request.
func NewGetBillingReq(botID, userID, limit, offset int64) *bot.GetBillingReq {
	return &bot.GetBillingReq{BotId: botID, UserId: userID, Limit: limit, Offset: offset}
}

// NewChatWithBotReq builds a bot chat request. conversationID is part of the
// memory key so the same bot can keep separate context per conversation.
func NewChatWithBotReq(botID, userID, conversationID int64, message string) *bot.ChatWithBotReq {
	return &bot.ChatWithBotReq{BotId: botID, UserId: userID, ConversationId: conversationID, Message: message}
}

// NewAgentTaskReq builds one context-aware Agent task request.
func NewAgentTaskReq(botID, userID, conversationID int64, question string) *bot.AgentTaskReq {
	return &bot.AgentTaskReq{BotId: botID, UserId: userID, ConversationId: conversationID, Question: question}
}

// NewGrantPermissionReq builds an Agent permission grant request.
func NewGrantPermissionReq(botID, operatorID, userID int64, role string) *bot.GrantPermissionReq {
	return &bot.GrantPermissionReq{BotId: botID, OperatorId: operatorID, UserId: userID, Role: role}
}

// NewRevokePermissionReq builds an Agent permission revoke request.
func NewRevokePermissionReq(botID, operatorID, userID int64) *bot.RevokePermissionReq {
	return &bot.RevokePermissionReq{BotId: botID, OperatorId: operatorID, UserId: userID}
}

// NewListPermissionsReq builds an Agent permission list request.
func NewListPermissionsReq(botID, operatorID int64) *bot.ListPermissionsReq {
	return &bot.ListPermissionsReq{BotId: botID, OperatorId: operatorID}
}
