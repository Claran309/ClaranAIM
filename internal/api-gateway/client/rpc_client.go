package client

import (
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

func NewCreateGroupReq(name string, ownerID int64, memberIDs []int64) *group.CreateGroupReq {
	return &group.CreateGroupReq{Name: name, OwnerId: ownerID, MemberIds: memberIDs}
}

func NewGetGroupReq(groupID int64) *group.GetGroupReq {
	return &group.GetGroupReq{GroupId: groupID}
}

func NewGetUserGroupsReq(userID int64) *group.GetUserGroupsReq {
	return &group.GetUserGroupsReq{UserId: userID}
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

func NewCreateConversationReq(convType string, participantIDs []int64) *message.CreateConversationReq {
	return &message.CreateConversationReq{Type: convType, ParticipantIds: participantIDs}
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
