// Package client 保存 api-gateway 侧的内部 RPC 客户端和请求构造器。
// 网关负责把浏览器 HTTP 请求转换为 Kitex RPC 调用；把生成请求结构的代码集中在这里，
// 可以让 handler 保持轻量，也避免 HTTP 绑定细节泄漏到各微服务客户端之外。
package client

import (
	"ClaranAIM/kitex_gen/bot"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"ClaranAIM/kitex_gen/file"
	"ClaranAIM/kitex_gen/file/fileservice"
	"ClaranAIM/kitex_gen/group"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/kitex_gen/knowledge/knowledgeservice"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/kitex_gen/message/historyservice"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"log"
	"sync"

	"github.com/cloudwego/kitex/client"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// 这些 Kitex 客户端在网关启动时初始化一次，并被各 HTTP handler 复用。
// 网关不持有下游数据库连接，所有业务读写都必须通过这些服务客户端完成。
var (
	once sync.Once

	// UserClient 调用 user-service，负责账号、个人资料、好友和在线状态相关能力。
	// 它在 api-gateway 启动阶段由 InitClients 初始化一次。
	UserClient userservice.Client
	// GroupClient 调用 group-service，负责群资料、群成员和群角色。
	GroupClient groupservice.Client
	// MessageClient 调用 msg-core-service，负责会话、消息写入、已读回执和本地消息状态。
	MessageClient messageservice.Client
	// HistoryClient 保留给历史消息服务的 RPC 契约。
	// 当前大部分历史读取走 msg-core-service，以便用户可见性规则集中在同一处。
	HistoryClient historyservice.Client
	// FileClient 调用 file-service，在网关把二进制内容写入本地或 MinIO 后保存文件元数据。
	FileClient fileservice.Client
	// AgentClient 调用 agent-manager-service，负责 Agent 管理、对话、路由和计费记录。
	// 生成代码包名暂时仍叫 bot，等 IDL 安全重生成后再统一迁移。
	AgentClient botservice.Client
	// AgentLongTaskClient 调用可能长时间阻塞的 Agent 方法，例如长思考或工具执行。
	// 它使用独立 agent_rpc 治理配置，避免普通 IM RPC 超时误杀长任务。
	AgentLongTaskClient botservice.Client
	// AgentRuntimeClient 调用 agent-runtime-service，读取运行时拥有的数据，例如长会话元信息。
	AgentRuntimeClient botruntimeservice.Client
	// MemoryClient 调用 memory-service，负责用户/群/会话记忆事实的治理和召回。
	MemoryClient memoryservice.Client
	// RAGClient 调用 rag-service，负责知识库入库、RAG 检索和 GraphRAG 子图读取。
	RAGClient ragservice.Client
	// KnowledgeClient 调用 knowledge-service，负责知识图谱查询、过滤和可视化视图。
	KnowledgeClient knowledgeservice.Client
	// SettingsClient 调用 settings-service，负责 LLM 预设、Prompt 和 Agent Skill 配置。
	SettingsClient settingsservice.Client
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
		agentCfg := config.RPCGovernanceConfig{}
		if len(rpcCfg) > 1 {
			agentCfg = rpcCfg[1]
		}
		baseOptions := append([]client.Option{client.WithResolver(r)}, governance.ClientOptions(cfg)...)
		agentOptions := append([]client.Option{client.WithResolver(r)}, governance.LongRunningClientOptions(agentCfg)...)

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

		AgentClient, err = botservice.NewClient("agent-manager-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建agent-manager-service客户端失败:", err)
		}

		AgentLongTaskClient, err = botservice.NewClient("agent-manager-service",
			agentOptions...,
		)
		if err != nil {
			log.Fatal("创建agent-manager-service长任务客户端失败:", err)
		}

		AgentRuntimeClient, err = botruntimeservice.NewClient("agent-runtime-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建agent-runtime-service客户端失败:", err)
		}

		MemoryClient, err = memoryservice.NewClient("memory-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建memory-service客户端失败:", err)
		}

		RAGClient, err = ragservice.NewClient("rag-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建rag-service客户端失败:", err)
		}

		KnowledgeClient, err = knowledgeservice.NewClient("knowledge-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建knowledge-service客户端失败:", err)
		}

		SettingsClient, err = settingsservice.NewClient("settings-service",
			baseOptions...,
		)
		if err != nil {
			log.Fatal("创建settings-service客户端失败:", err)
		}

		log.Println("RPC客户端初始化成功")
	})
}

// NewRegisterReq 根据 HTTP 表单字段构造 user-service 注册请求。
func NewRegisterReq(username, password, nickname string) *user.RegisterReq {
	return &user.RegisterReq{Username: username, Password: password, Nickname: nickname}
}

// NewRegisterSystemUserReq 构造系统用户注册请求。
// Agent 等内部身份使用这种不可直接登录的账号，普通浏览器注册必须继续走 NewRegisterReq。
func NewRegisterSystemUserReq(username, password, nickname string) *user.RegisterReq {
	return &user.RegisterReq{Username: username, Password: password, Nickname: nickname, IsSystem: true}
}

// NewLoginReq 构造登录请求。
// 密码校验留在 user-service 内部，网关不负责哈希或保存密码。
func NewLoginReq(username, password string) *user.LoginReq {
	return &user.LoginReq{Username: username, Password: password}
}

// NewGetUserInfoReq 构造用户资料查询请求，供个人页、好友列表和消息发送者展示使用。
func NewGetUserInfoReq(userID int64) *user.GetUserInfoReq {
	return &user.GetUserInfoReq{UserId: userID}
}

// NewUpdateUserInfoReq 构造用户资料更新请求。
// fullUpdate 用于告诉 user-service：空字符串是主动清空字段，还是应被忽略。
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

// NewAddFriendReq 构造添加好友请求。
// userID 必须来自当前登录态，不能信任客户端传入的操作者 ID。
func NewAddFriendReq(userID, friendID, groupID int64, remark string) *user.AddFriendReq {
	return &user.AddFriendReq{UserId: userID, FriendId: friendID, GroupId: groupID, Remark: remark}
}

// NewDeleteFriendReq 构造双向删除好友请求。
func NewDeleteFriendReq(userID, friendID int64) *user.DeleteFriendReq {
	return &user.DeleteFriendReq{UserId: userID, FriendId: friendID}
}

// NewUpdateFriendRemarkReq 构造好友备注或分组变更请求。
func NewUpdateFriendRemarkReq(userID, friendID, groupID int64, remark string) *user.UpdateFriendRemarkReq {
	return &user.UpdateFriendRemarkReq{UserId: userID, FriendId: friendID, Remark: remark, GroupId: groupID}
}

// NewGetFriendListReq 构造侧边栏好友列表查询请求。
func NewGetFriendListReq(userID int64) *user.GetFriendListReq {
	return &user.GetFriendListReq{UserId: userID}
}

// NewBatchGetUserInfoReq 构造批量资料查询请求，避免可见消息逐条 RPC 查询昵称头像。
func NewBatchGetUserInfoReq(ids []int64) *user.BatchGetUserInfoReq {
	return &user.BatchGetUserInfoReq{UserIds: ids}
}

// 下面的 New...Req 辅助函数刻意不放业务规则。
// 它们只把 handler 已校验的基础值适配成 Thrift 结构体；权限、成员关系和持久化仍由所属微服务负责。

// NewCreateGroupReq 构造 group-service 创建群请求。
func NewCreateGroupReq(name string, ownerID int64, memberIDs []int64) *group.CreateGroupReq {
	return &group.CreateGroupReq{Name: name, OwnerId: ownerID, MemberIds: memberIDs}
}

// NewDeleteGroupReq 构造删除群请求，operator 使用当前登录用户。
func NewDeleteGroupReq(groupID, operatorID int64) *group.DeleteGroupReq {
	return &group.DeleteGroupReq{GroupId: groupID, OperatorId: operatorID}
}

// NewGetGroupReq 构造群资料查询请求。
func NewGetGroupReq(groupID int64) *group.GetGroupReq {
	return &group.GetGroupReq{GroupId: groupID}
}

// NewGetUserGroupsReq 构造当前用户群列表查询请求。
func NewGetUserGroupsReq(userID int64) *group.GetUserGroupsReq {
	return &group.GetUserGroupsReq{UserId: userID}
}

// NewUpdateGroupReq 构造群资料更新请求。
// 当网关确认 announcement 字段显式出现时，空字符串表示清空公告。
func NewUpdateGroupReq(groupID, operatorID int64, name, announcement string) *group.UpdateGroupReq {
	return &group.UpdateGroupReq{GroupId: groupID, OperatorId: operatorID, Name: name, Announcement: announcement}
}

// NewInviteMemberReq 构造邀请群成员请求。
func NewInviteMemberReq(groupID, operatorID int64, userIDs []int64) *group.InviteMemberReq {
	return &group.InviteMemberReq{GroupId: groupID, OperatorId: operatorID, UserIds: userIDs}
}

// NewKickMemberReq 构造移除群成员请求。
func NewKickMemberReq(groupID, operatorID, userID int64) *group.KickMemberReq {
	return &group.KickMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID}
}

// NewGetGroupMembersReq 构造群成员列表请求。
func NewGetGroupMembersReq(groupID int64) *group.GetGroupMembersReq {
	return &group.GetGroupMembersReq{GroupId: groupID}
}

// NewTransferOwnerReq 构造群主转让请求。
func NewTransferOwnerReq(groupID, operatorID, newOwnerID int64) *group.TransferOwnerReq {
	return &group.TransferOwnerReq{GroupId: groupID, OperatorId: operatorID, NewOwnerId_: newOwnerID}
}

// NewPinGroupReq 构造当前用户的群置顶/取消置顶请求。
func NewPinGroupReq(groupID, operatorID int64, isPinned bool) *group.PinGroupReq {
	return &group.PinGroupReq{GroupId: groupID, OperatorId: operatorID, IsPinned: isPinned}
}

// NewMuteMemberReq 构造群成员定时禁言请求。
func NewMuteMemberReq(groupID, operatorID, userID, durationMinutes int64) *group.MuteMemberReq {
	return &group.MuteMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID, DurationMinutes: durationMinutes}
}

// NewUnmuteMemberReq 构造解除禁言请求。
func NewUnmuteMemberReq(groupID, operatorID, userID int64) *group.UnmuteMemberReq {
	return &group.UnmuteMemberReq{GroupId: groupID, OperatorId: operatorID, UserId: userID}
}

// NewSetRoleReq 构造群角色更新请求。
func NewSetRoleReq(groupID, operatorID, userID int64, role string) *group.SetRoleReq {
	return &group.SetRoleReq{GroupId: groupID, OperatorId: operatorID, UserId: userID, Role: role}
}

// NewCheckMemberReq 构造群成员身份检查请求。
func NewCheckMemberReq(groupID, userID int64) *group.CheckMemberReq {
	return &group.CheckMemberReq{GroupId: groupID, UserId: userID}
}

// NewCreateConversationReq 构造 msg-core-service 创建会话请求。
func NewCreateConversationReq(convType string, participantIDs []int64, groupID int64) *message.CreateConversationReq {
	return &message.CreateConversationReq{Type: convType, ParticipantIds: participantIDs, GroupId: groupID}
}

// NewSendMessageReq 构造普通消息发送请求。
func NewSendMessageReq(conversationID, senderID int64, content, msgType string) *message.SendMessageReq {
	return NewSendMessageExtReq(conversationID, senderID, content, msgType, 0, nil, false)
}

// NewSendMessageExtReq 构造带引用和 @ 元数据的消息发送请求。
// 普通文本、媒体引用和广播消息都复用这一入口。
func NewSendMessageExtReq(conversationID, senderID int64, content, msgType string, replyToID int64, mentionUserIDs []int64, mentionAll bool) *message.SendMessageReq {
	return NewSendMessageExtReqWithClientID(conversationID, senderID, content, msgType, replyToID, mentionUserIDs, mentionAll, "")
}

// NewSendMessageExtReqWithClientID 在消息发送请求上附加可选幂等键。
// Agent Dispatcher 等内部生产者应使用来源事件推导稳定 client_msg_id，避免 Kafka 重试生成重复回复。
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

// NewGetHistoryReq 构造当前用户可见视角下的历史消息查询请求。
func NewGetHistoryReq(conversationID, userID, limit, beforeID int64) *message.GetHistoryReq {
	return &message.GetHistoryReq{ConversationId: conversationID, UserId: userID, Limit: limit, BeforeId: beforeID}
}

// NewDeleteLocalMessageReq 构造“仅对当前用户隐藏消息”的请求，不删除全局消息事实。
func NewDeleteLocalMessageReq(conversationID, userID, messageID int64) *message.DeleteLocalMessageReq {
	return &message.DeleteLocalMessageReq{ConversationId: conversationID, UserId: userID, MessageId: messageID}
}

// NewSearchMessagesReq 构造用户范围内的历史消息搜索请求。
func NewSearchMessagesReq(userID int64, keyword string, limit int64) *message.SearchMessagesReq {
	return &message.SearchMessagesReq{UserId: userID, Keyword: keyword, Limit: limit}
}

// NewSearchMessagesInConvReq 构造指定会话范围内的消息搜索请求。
func NewSearchMessagesInConvReq(conversationIDs []int64, keyword string, limit int64) *message.SearchMessagesReq {
	return &message.SearchMessagesReq{ConversationIds: conversationIDs, Keyword: keyword, Limit: limit}
}

// NewSearchMessagesAdvancedReq 构造带可选时间范围的高级搜索请求。
func NewSearchMessagesAdvancedReq(userID int64, conversationIDs []int64, keyword string, limit int64, startAt, endAt string) *message.SearchMessagesReq {
	return &message.SearchMessagesReq{UserId: userID, ConversationIds: conversationIDs, Keyword: keyword, Limit: limit, StartAt: startAt, EndAt: endAt}
}

// NewUploadFileReq 构造文件元数据写入请求，二进制内容应已由网关写入本地或 MinIO。
func NewUploadFileReq(fileName, fileType string, fileSize int64, contentType, fileURL string, uploaderID int64) *file.UploadFileReq {
	return &file.UploadFileReq{FileName: fileName, FileType: fileType, FileSize: fileSize, ContentType: contentType, FileUrl: fileURL, UploaderId: uploaderID}
}

// NewGetFileReq 构造文件元数据查询请求。
func NewGetFileReq(fileID string) *file.GetFileReq {
	return &file.GetFileReq{FileId: fileID}
}

// NewDeleteFileReq 构造带操作者身份的文件元数据删除请求。
func NewDeleteFileReq(fileID string, operatorID int64) *file.DeleteFileReq {
	return &file.DeleteFileReq{FileId: fileID, OperatorId: operatorID}
}

// NewListFilesReq 构造分页文件元数据列表请求。
func NewListFilesReq(uploaderID int64, fileType string, limit, offset int64) *file.ListFilesReq {
	return &file.ListFilesReq{UploaderId: uploaderID, FileType: fileType, Limit: limit, Offset: offset}
}

// NewCreateAgentReq 构造 Agent 创建请求，包含创建者和模型配置。
func NewCreateAgentReq(name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, ownerID, contextMessageLimit, memoryRecallLimit, maxOutputTokens int64, temperature float64, groupTriggerMode string, autoReplyEnabled bool) *bot.CreateBotReq {
	return &bot.CreateBotReq{Name: name, Type: botType, Description: description, ModelName: modelName, ApiKey: apiKey, BaseUrl: baseURL, SystemPrompt: systemPrompt, SkillsDir: skillsDir, AgentRoot: agentRoot, Avatar: avatar, Signature: signature, WorkspaceRoot: workspaceRoot, ToolPolicy: toolPolicy, OwnerId: ownerID, ContextMessageLimit: contextMessageLimit, MemoryRecallLimit: memoryRecallLimit, MaxOutputTokens: maxOutputTokens, Temperature: temperature, GroupTriggerMode: groupTriggerMode, AutoReplyEnabled: autoReplyEnabled}
}

// NewUpdateAgentReq 构造 Agent 更新请求。
// 空密钥字段由 agent-manager-service 判断含义，网关不擅自决定保留或清空。
func NewUpdateAgentReq(botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, isActive bool, isActiveSet bool, contextMessageLimit, memoryRecallLimit, maxOutputTokens int64, temperature float64, groupTriggerMode string, autoReplyEnabled bool) *bot.UpdateBotReq {
	return &bot.UpdateBotReq{BotId: botID, OperatorId: operatorID, Name: name, Description: description, ModelName: modelName, ApiKey: apiKey, BaseUrl: baseURL, SystemPrompt: systemPrompt, SkillsDir: skillsDir, AgentRoot: agentRoot, Avatar: avatar, Signature: signature, WorkspaceRoot: workspaceRoot, ToolPolicy: toolPolicy, IsActive: isActive, IsActiveSet: isActiveSet, ContextMessageLimit: contextMessageLimit, MemoryRecallLimit: memoryRecallLimit, MaxOutputTokens: maxOutputTokens, Temperature: temperature, GroupTriggerMode: groupTriggerMode, AutoReplyEnabled: autoReplyEnabled}
}

// NewGetAgentReq 构造 Agent 元数据查询请求。
func NewGetAgentReq(botID int64) *bot.GetBotReq {
	return &bot.GetBotReq{BotId: botID}
}

// NewListAgentsReq 构造当前用户范围内的 Agent 列表请求。
func NewListAgentsReq(ownerID int64, botType string) *bot.ListBotsReq {
	return &bot.ListBotsReq{OwnerId: ownerID, Type: botType}
}

// NewDeleteAgentReq 构造带操作者身份的 Agent 删除请求。
func NewDeleteAgentReq(botID, operatorID int64) *bot.DeleteBotReq {
	return &bot.DeleteBotReq{BotId: botID, OperatorId: operatorID}
}

// NewCreateRouteReq 构造 Agent 路由规则创建请求。
func NewCreateRouteReq(botID int64, routePattern, routeType string, priority int64) *bot.CreateRouteReq {
	return &bot.CreateRouteReq{BotId: botID, RoutePattern: routePattern, RouteType: routeType, Priority: priority}
}

// NewListRoutesReq 构造 Agent 路由规则列表请求。
func NewListRoutesReq(botID int64) *bot.ListRoutesReq {
	return &bot.ListRoutesReq{BotId: botID}
}

// NewDeleteRouteReq 构造 Agent 路由规则删除请求。
func NewDeleteRouteReq(routeID, operatorID int64) *bot.DeleteRouteReq {
	return &bot.DeleteRouteReq{RouteId: routeID, OperatorId: operatorID}
}

// NewGetBillingReq 构造分页计费记录查询请求。
func NewGetBillingReq(botID, userID, limit, offset int64) *bot.GetBillingReq {
	return &bot.GetBillingReq{BotId: botID, UserId: userID, Limit: limit, Offset: offset}
}

// NewChatWithAgentReq 构造 Agent 对话请求。
// conversationID 会进入记忆 key，使同一个 Agent 在不同 IM 会话中保持独立上下文。
func NewChatWithAgentReq(botID, userID, conversationID int64, message string) *bot.ChatWithBotReq {
	return &bot.ChatWithBotReq{BotId: botID, UserId: userID, ConversationId: conversationID, Message: message}
}

// NewAgentTaskReq 构造一次上下文感知的 Agent 任务请求。
func NewAgentTaskReq(botID, userID, conversationID int64, question string) *bot.AgentTaskReq {
	return &bot.AgentTaskReq{BotId: botID, UserId: userID, ConversationId: conversationID, Question: question}
}

// NewGrantPermissionReq 构造 Agent 权限授予请求。
func NewGrantPermissionReq(botID, operatorID, userID int64, role string) *bot.GrantPermissionReq {
	return &bot.GrantPermissionReq{BotId: botID, OperatorId: operatorID, UserId: userID, Role: role}
}

// NewRevokePermissionReq 构造 Agent 权限撤销请求。
func NewRevokePermissionReq(botID, operatorID, userID int64) *bot.RevokePermissionReq {
	return &bot.RevokePermissionReq{BotId: botID, OperatorId: operatorID, UserId: userID}
}

// NewListPermissionsReq 构造 Agent 权限列表查询请求。
func NewListPermissionsReq(botID, operatorID int64) *bot.ListPermissionsReq {
	return &bot.ListPermissionsReq{BotId: botID, OperatorId: operatorID}
}
