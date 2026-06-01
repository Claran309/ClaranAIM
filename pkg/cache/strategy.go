package cache

import (
	"fmt"
	"time"
)

// Policy 描述一类缓存的 key、TTL、抖动和空值策略。
// 业务层统一使用这些策略，避免不同服务各自写散落的 magic string。
type Policy struct {
	Key        string
	TTL        time.Duration
	Jitter     time.Duration
	NullTTL    time.Duration
	NullJitter time.Duration
}

func UserInfoPolicy(userID int64) Policy {
	return Policy{Key: fmt.Sprintf("user:info:%d", userID), TTL: 15 * time.Minute, Jitter: time.Minute, NullTTL: time.Minute, NullJitter: 30 * time.Second}
}

func UserFriendsPolicy(userID int64) Policy {
	return Policy{Key: fmt.Sprintf("user:friends:%d", userID), TTL: 5 * time.Minute, Jitter: 30 * time.Second, NullTTL: time.Minute, NullJitter: 30 * time.Second}
}

func OnlineUserPolicy(userID int64) Policy {
	return Policy{Key: fmt.Sprintf("online:user:%d", userID), TTL: 30 * time.Minute, Jitter: 2 * time.Minute}
}

func FriendGroupsPolicy(userID int64) Policy {
	return Policy{Key: fmt.Sprintf("user:friend_groups:%d", userID), TTL: 10 * time.Minute, Jitter: time.Minute, NullTTL: time.Minute, NullJitter: 30 * time.Second}
}

func GroupInfoPolicy(groupID int64) Policy {
	return Policy{Key: fmt.Sprintf("group:info:%d", groupID), TTL: 15 * time.Minute, Jitter: time.Minute, NullTTL: time.Minute, NullJitter: 30 * time.Second}
}

func GroupMembersPolicy(groupID int64) Policy {
	return Policy{Key: fmt.Sprintf("group:members:%d", groupID), TTL: 10 * time.Minute, Jitter: time.Minute, NullTTL: time.Minute, NullJitter: 30 * time.Second}
}

func UserGroupsPolicy(userID int64) Policy {
	return Policy{Key: fmt.Sprintf("user:groups:%d", userID), TTL: 5 * time.Minute, Jitter: 30 * time.Second, NullTTL: time.Minute, NullJitter: 30 * time.Second}
}

func UserConversationsPolicy(userID int64) Policy {
	return Policy{Key: fmt.Sprintf("user:conversations:%d", userID), TTL: 5 * time.Minute, Jitter: 30 * time.Second, NullTTL: time.Minute, NullJitter: 30 * time.Second}
}

func RecentConversationMessagesPolicy(conversationID int64) Policy {
	return Policy{Key: fmt.Sprintf("conversation:recent:%d", conversationID), TTL: 10 * time.Minute, Jitter: time.Minute}
}

func ConversationListKeys(userIDs []int64) []string {
	seen := make(map[int64]struct{}, len(userIDs))
	keys := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		keys = append(keys, UserConversationsPolicy(userID).Key)
	}
	return keys
}
