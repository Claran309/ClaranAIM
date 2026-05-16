package service

import (
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/kitex_gen/group"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/kitex/client/callopt"
)

type fakeMessageRepo struct {
	nextConversationID int64
	nextMessageID      int64
	conversations      map[int64]*model.Conversation
	participants       map[int64]map[int64]model.ConversationParticipant
	messages           map[int64][]model.Message
	editRecords        []model.MessageEditRecord
}

func newFakeMessageRepo() *fakeMessageRepo {
	return &fakeMessageRepo{
		nextConversationID: 1,
		nextMessageID:      1,
		conversations:      map[int64]*model.Conversation{},
		participants:       map[int64]map[int64]model.ConversationParticipant{},
		messages:           map[int64][]model.Message{},
		editRecords:        []model.MessageEditRecord{},
	}
}

func (r *fakeMessageRepo) CreateConversation(ctx context.Context, conv *model.Conversation) error {
	if conv.ID == 0 {
		conv.ID = r.nextConversationID
		r.nextConversationID++
	}
	now := time.Now()
	conv.CreatedAt = now
	conv.UpdatedAt = now
	cp := *conv
	r.conversations[conv.ID] = &cp
	return nil
}

func (r *fakeMessageRepo) GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error) {
	conv := r.conversations[id]
	if conv == nil {
		return nil, nil
	}
	cp := *conv
	return &cp, nil
}

func (r *fakeMessageRepo) UpdateConversation(ctx context.Context, conv *model.Conversation) error {
	cp := *conv
	r.conversations[conv.ID] = &cp
	return nil
}

func (r *fakeMessageRepo) AddParticipant(ctx context.Context, p *model.ConversationParticipant) error {
	if r.participants[p.ConversationID] == nil {
		r.participants[p.ConversationID] = map[int64]model.ConversationParticipant{}
	}
	if _, ok := r.participants[p.ConversationID][p.UserID]; ok {
		return nil
	}
	cp := *p
	cp.ID = int64(len(r.participants[p.ConversationID]) + 1)
	cp.JoinedAt = time.Now()
	cp.NotifyEnabled = true
	r.participants[p.ConversationID][p.UserID] = cp
	return nil
}

func (r *fakeMessageRepo) GetParticipants(ctx context.Context, conversationID int64) ([]model.ConversationParticipant, error) {
	var result []model.ConversationParticipant
	for _, p := range r.participants[conversationID] {
		result = append(result, p)
	}
	return result, nil
}

func (r *fakeMessageRepo) GetUserConversations(ctx context.Context, userID int64) ([]model.ConversationParticipant, error) {
	var result []model.ConversationParticipant
	for _, byUser := range r.participants {
		if p, ok := byUser[userID]; ok {
			result = append(result, p)
		}
	}
	return result, nil
}

func (r *fakeMessageRepo) CreateMessage(ctx context.Context, msg *model.Message) error {
	if msg.ID == 0 {
		msg.ID = r.nextMessageID
		r.nextMessageID++
	}
	msg.CreatedAt = time.Now()
	r.messages[msg.ConversationID] = append(r.messages[msg.ConversationID], *msg)
	return nil
}

func (r *fakeMessageRepo) GetMessageByID(ctx context.Context, messageID int64) (*model.Message, error) {
	for _, msgs := range r.messages {
		for _, msg := range msgs {
			if msg.ID == messageID {
				cp := msg
				return &cp, nil
			}
		}
	}
	return nil, nil
}

func (r *fakeMessageRepo) UpdateMessage(ctx context.Context, msg *model.Message) error {
	for convID, msgs := range r.messages {
		for i := range msgs {
			if msgs[i].ID == msg.ID {
				r.messages[convID][i] = *msg
				return nil
			}
		}
	}
	return nil
}

func (r *fakeMessageRepo) CreateEditRecord(ctx context.Context, record *model.MessageEditRecord) error {
	cp := *record
	cp.ID = int64(len(r.editRecords) + 1)
	cp.CreatedAt = time.Now()
	r.editRecords = append(r.editRecords, cp)
	return nil
}

func (r *fakeMessageRepo) UpdateParticipantReadCursor(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error {
	p := r.participants[conversationID][userID]
	p.LastReadMessageID = messageID
	p.LastReadAt = readAt
	r.participants[conversationID][userID] = p
	return nil
}

func (r *fakeMessageRepo) UpdateParticipantSettings(ctx context.Context, conversationID, userID int64, draft *string, isPinned *bool, notifyEnabled *bool) error {
	p := r.participants[conversationID][userID]
	if draft != nil {
		p.Draft = *draft
	}
	if isPinned != nil {
		p.IsPinned = *isPinned
	}
	if notifyEnabled != nil {
		p.NotifyEnabled = *notifyEnabled
	}
	r.participants[conversationID][userID] = p
	return nil
}

func (r *fakeMessageRepo) GetMessages(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.Message, error) {
	msgs := r.messages[conversationID]
	if beforeID > 0 {
		filtered := make([]model.Message, 0, len(msgs))
		for _, msg := range msgs {
			if msg.ID < beforeID {
				filtered = append(filtered, msg)
			}
		}
		msgs = filtered
	}
	if limit > 0 && int64(len(msgs)) > limit {
		msgs = msgs[len(msgs)-int(limit):]
	}
	out := make([]model.Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

func (r *fakeMessageRepo) SearchMessages(ctx context.Context, conversationIDs []int64, keyword string, limit int64, startAt, endAt *time.Time) ([]model.Message, error) {
	allowed := map[int64]struct{}{}
	for _, id := range conversationIDs {
		allowed[id] = struct{}{}
	}
	var result []model.Message
	for convID, msgs := range r.messages {
		if _, ok := allowed[convID]; !ok {
			continue
		}
		for _, msg := range msgs {
			if !strings.Contains(msg.Content, keyword) {
				continue
			}
			if startAt != nil && msg.CreatedAt.Before(*startAt) {
				continue
			}
			if endAt != nil && msg.CreatedAt.After(*endAt) {
				continue
			}
			result = append(result, msg)
		}
	}
	if limit > 0 && int64(len(result)) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *fakeMessageRepo) FindPrivateConversation(ctx context.Context, userID1, userID2 int64) (*model.Conversation, error) {
	for _, conv := range r.conversations {
		if conv.Type != "private" {
			continue
		}
		byUser := r.participants[conv.ID]
		if _, ok := byUser[userID1]; !ok {
			continue
		}
		if _, ok := byUser[userID2]; ok {
			cp := *conv
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeMessageRepo) FindGroupConversation(ctx context.Context, groupID int64) (*model.Conversation, error) {
	for _, conv := range r.conversations {
		if conv.Type == "group" && conv.GroupID == groupID {
			cp := *conv
			return &cp, nil
		}
	}
	return nil, nil
}

type fakeGroupClient struct {
	membersResp *group.GetGroupMembersResp
	membersErr  error
	groupResp   *group.GetGroupResp
	groupErr    error
}

func (c *fakeGroupClient) CreateGroup(ctx context.Context, req *group.CreateGroupReq, callOptions ...callopt.Option) (*group.CreateGroupResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) DeleteGroup(ctx context.Context, req *group.DeleteGroupReq, callOptions ...callopt.Option) (*group.DeleteGroupResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) UpdateGroup(ctx context.Context, req *group.UpdateGroupReq, callOptions ...callopt.Option) (*group.UpdateGroupResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) GetGroup(ctx context.Context, req *group.GetGroupReq, callOptions ...callopt.Option) (*group.GetGroupResp, error) {
	return c.groupResp, c.groupErr
}

func (c *fakeGroupClient) GetUserGroups(ctx context.Context, req *group.GetUserGroupsReq, callOptions ...callopt.Option) (*group.GetUserGroupsResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) InviteMember(ctx context.Context, req *group.InviteMemberReq, callOptions ...callopt.Option) (*group.InviteMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) KickMember(ctx context.Context, req *group.KickMemberReq, callOptions ...callopt.Option) (*group.KickMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) MuteMember(ctx context.Context, req *group.MuteMemberReq, callOptions ...callopt.Option) (*group.MuteMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) UnmuteMember(ctx context.Context, req *group.UnmuteMemberReq, callOptions ...callopt.Option) (*group.UnmuteMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) SetRole(ctx context.Context, req *group.SetRoleReq, callOptions ...callopt.Option) (*group.SetRoleResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) GetGroupMembers(ctx context.Context, req *group.GetGroupMembersReq, callOptions ...callopt.Option) (*group.GetGroupMembersResp, error) {
	return c.membersResp, c.membersErr
}

func (c *fakeGroupClient) CheckMember(ctx context.Context, req *group.CheckMemberReq, callOptions ...callopt.Option) (*group.CheckMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) TransferOwner(ctx context.Context, req *group.TransferOwnerReq, callOptions ...callopt.Option) (*group.TransferOwnerResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) PinGroup(ctx context.Context, req *group.PinGroupReq, callOptions ...callopt.Option) (*group.PinGroupResp, error) {
	return nil, nil
}

func TestCreateConversationRejectsGroupWithoutGroupID(t *testing.T) {
	svc := &messageServiceImpl{repo: newFakeMessageRepo()}

	_, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2}, 0)
	if err == nil || !strings.Contains(err.Error(), "group_id") {
		t.Fatalf("expected group_id validation error, got %v", err)
	}
}

func TestCreateConversationSyncsParticipantsForExistingGroupConversation(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}

	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2}, 10)
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	existing, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 3}, 10)
	if err != nil {
		t.Fatalf("reuse group conversation: %v", err)
	}
	if existing.ID != conv.ID {
		t.Fatalf("expected existing conversation %d, got %d", conv.ID, existing.ID)
	}

	participants, err := repo.GetParticipants(context.Background(), conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(participants) != 3 {
		t.Fatalf("expected 3 synced participants, got %d", len(participants))
	}
}

func TestGetHistoryRejectsNonParticipant(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	repo.messages[conv.ID] = []model.Message{{ID: 1, ConversationID: conv.ID, SenderID: 1, Content: "secret"}}

	_, err = svc.GetHistory(context.Background(), conv.ID, 99, 50, 0)
	if !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("expected access denied error, got %v", err)
	}
}

func TestSendMessageRejectsNonParticipant(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.SendMessage(context.Background(), conv.ID, 99, "hello", "text")
	if !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("expected access denied error, got %v", err)
	}
	if got := len(repo.messages[conv.ID]); got != 0 {
		t.Fatalf("expected no message to be stored, got %d", got)
	}
}

func TestSendMessageRejectsDeletedGroupEvenIfConversationParticipantRemains(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{
		repo: repo,
		groupClient: &fakeGroupClient{membersResp: &group.GetGroupMembersResp{
			Success: false,
			Msg:     "群组不存在",
		}},
	}
	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 3}, 10)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.SendMessage(context.Background(), conv.ID, 2, "still here?", "text")
	if !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("expected access denied for deleted group, got %v", err)
	}
	if got := len(repo.messages[conv.ID]); got != 0 {
		t.Fatalf("expected no fake group message to be stored, got %d", got)
	}
}

func TestGetUserConversationsSkipsDeletedGroupConversation(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{
		repo: repo,
		groupClient: &fakeGroupClient{groupResp: &group.GetGroupResp{
			Success: false,
			Msg:     "群组不存在",
		}},
	}
	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 3}, 10)
	if err != nil {
		t.Fatal(err)
	}
	repo.messages[conv.ID] = []model.Message{{ID: 1, ConversationID: conv.ID, SenderID: 2, Content: "old"}}

	convs, err := svc.GetUserConversations(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 1 {
		t.Fatalf("expected deleted group conversation placeholder, got %d", len(convs))
	}
	if !convs[0].IsDeletedGroup {
		t.Fatal("expected deleted group conversation to be marked")
	}
	if !strings.Contains(convs[0].TargetName, "已解散") {
		t.Fatalf("expected deleted group label, got %q", convs[0].TargetName)
	}
}

func TestSendMessageStoresReplyMentionsAndBroadcastType(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	root, err := svc.SendMessage(context.Background(), conv.ID, 1, "root", "text")
	if err != nil {
		t.Fatal(err)
	}

	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       2,
		Content:        "reply @alice",
		MsgType:        "broadcast",
		ReplyToID:      root.ID,
		MentionUserIDs: []int64{1, 1, 0},
	})
	if err != nil {
		t.Fatalf("send ext: %v", err)
	}
	if msg.ReplyToID != root.ID {
		t.Fatalf("expected reply_to_id %d, got %d", root.ID, msg.ReplyToID)
	}
	if msg.MsgType != "broadcast" {
		t.Fatalf("expected broadcast msg type, got %q", msg.MsgType)
	}
	if len(msg.MentionUserIDs) != 1 || msg.MentionUserIDs[0] != 1 {
		t.Fatalf("expected deduped mentions [1], got %#v", msg.MentionUserIDs)
	}
}

func TestMarkReadUpdatesParticipantCursorAndUnreadCount(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendMessage(context.Background(), conv.ID, 1, "one", "text"); err != nil {
		t.Fatal(err)
	}
	second, err := svc.SendMessage(context.Background(), conv.ID, 1, "two", "text")
	if err != nil {
		t.Fatal(err)
	}

	convs, err := svc.GetUserConversations(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 1 || convs[0].UnreadCount != 2 {
		t.Fatalf("expected unread count 2 before read, got %#v", convs)
	}

	if err := svc.MarkConversationRead(context.Background(), conv.ID, 2, second.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	convs, err = svc.GetUserConversations(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 1 || convs[0].UnreadCount != 0 {
		t.Fatalf("expected unread count 0 after read, got %#v", convs)
	}
}

func TestEditAndRecallMessageRespectSenderAndTimeLimit(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo, recallWindow: time.Hour}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "before", "text")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.EditMessage(context.Background(), msg.ID, 2, "bad"); !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("expected non-sender edit denied, got %v", err)
	}
	edited, err := svc.EditMessage(context.Background(), msg.ID, 1, "after")
	if err != nil {
		t.Fatalf("edit by sender: %v", err)
	}
	if edited.Content != "after" || !edited.IsEdited {
		t.Fatalf("expected edited message, got %#v", edited)
	}

	if err := svc.RecallMessage(context.Background(), msg.ID, 1); err != nil {
		t.Fatalf("recall by sender: %v", err)
	}
	recalled, err := repo.GetMessageByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recalled.Status != MessageStatusRecalled || recalled.Content != "" {
		t.Fatalf("expected recalled empty message, got %#v", recalled)
	}
}

func TestSearchMessagesSupportsTimeRangeAndCurrentConversation(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendMessage(context.Background(), conv.ID, 1, "alpha old", "text"); err != nil {
		t.Fatal(err)
	}
	repo.messages[conv.ID][0].CreatedAt = time.Now().Add(-48 * time.Hour)
	if _, err := svc.SendMessage(context.Background(), conv.ID, 1, "alpha new", "text"); err != nil {
		t.Fatal(err)
	}

	msgs, err := svc.SearchMessagesAdvanced(context.Background(), SearchMessagesOptions{
		UserID:          2,
		ConversationIDs: []int64{conv.ID},
		Keyword:         "alpha",
		Limit:           10,
		StartAt:         time.Now().Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Content != "alpha new" {
		t.Fatalf("expected only recent alpha message, got %#v", msgs)
	}
}
